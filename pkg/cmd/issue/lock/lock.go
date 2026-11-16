// Package lock locks and unlocks the conversation of an issue, as `gh issue
// lock` and `gh issue unlock` do.
package lock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// reasons maps gh's lock reasons to the default lock reasons of Gitea
// (`[repository.issue] LOCK_REASONS`), which its web UI shows.
var reasons = map[string]string{
	"off_topic":  "Off-topic",
	"resolved":   "Resolved",
	"spam":       "Spam",
	"too_heated": "Too heated",
}

const reasonsString = "off_topic, resolved, spam, too_heated"

// LockOptions holds all inputs for the issue lock and unlock commands.
type LockOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Reason     string
}

func (opts *LockOptions) issue() string {
	return fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)
}

// NewCmdLock creates the `copia issue lock` command.
func NewCmdLock(f *cmdutil.Factory) *cobra.Command {
	opts := &LockOptions{}

	cmd := &cobra.Command{
		Use:   "lock {<number> | <url>}",
		Short: "Lock issue conversation",
		Long: "Lock the conversation of an issue by number or URL, so that only collaborators can comment.\n\n" +
			"An issue that is already locked is not changed; unlock it first to lock it with another\n" +
			"reason. Locking needs write access to the repository's issues.\n\n" +
			"app.copia.io does not serve the lock endpoint and answers with HTTP 404, so the command\n" +
			"reports that the host does not support it.",
		Example: `  # Lock an issue in the current repository
  $ copia-cli issue lock 23

  # Lock an issue as resolved
  $ copia-cli issue lock 23 --reason resolved

  # Lock an issue by URL
  $ copia-cli issue lock https://app.copia.io/my-org/my-repo/issues/23`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkReason(opts.Reason); err != nil {
				return err
			}
			return runTargeted(f, opts, args[0], LockRun)
		},
	}

	cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", fmt.Sprintf("Optional reason for locking conversation (%s)", reasonsString))
	return cmd
}

// NewCmdUnlock creates the `copia issue unlock` command.
func NewCmdUnlock(f *cmdutil.Factory) *cobra.Command {
	opts := &LockOptions{}

	return &cobra.Command{
		Use:   "unlock {<number> | <url>}",
		Short: "Unlock issue conversation",
		Long: "Unlock the conversation of an issue by number or URL.\n\n" +
			"An issue that is not locked is left as it is. Unlocking needs write access to the\n" +
			"repository's issues.\n\n" +
			"app.copia.io does not serve the lock endpoint and answers with HTTP 404, so the command\n" +
			"reports that the host does not support it.",
		Example: `  # Unlock an issue in the current repository
  $ copia-cli issue unlock 23

  # Unlock an issue in a specific repository
  $ copia-cli issue unlock 23 --repo my-org/my-repo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTargeted(f, opts, args[0], UnlockRun)
		},
	}
}

// runTargeted resolves the issue argument, the auth and the repository into
// opts and calls run.
func runTargeted(f *cmdutil.Factory, opts *LockOptions, arg string, run func(*LockOptions) error) error {
	ref, err := shared.ParseIssueArg(arg)
	if err != nil {
		return err
	}
	opts.Number = ref.Number
	opts.IO = f.IOStreams

	host, token, err := f.ResolveAuth()
	if err != nil {
		return err
	}
	opts.Host = host
	opts.Token = token

	var owner, repo string
	if ref.Host == "" {
		owner, repo, err = f.ResolveRepo()
		if err != nil {
			return err
		}
	}
	opts.Owner, opts.Repo, err = ref.Resolve(host, owner, repo)
	if err != nil {
		return err
	}
	opts.HTTPClient = cmdutil.NewHTTPClient()
	return run(opts)
}

func checkReason(reason string) error {
	if _, ok := reasons[reason]; reason != "" && !ok {
		return fmt.Errorf("invalid reason %q; use one of %s", reason, reasonsString)
	}
	return nil
}

// LockRun locks the issue with PUT /repos/{owner}/{repo}/issues/{index}/lock.
func LockRun(opts *LockOptions) error {
	if err := checkReason(opts.Reason); err != nil {
		return err
	}
	state, err := fetchLockState(opts)
	if err != nil {
		return err
	}
	if state.Locked {
		// Gitea ignores a lock request for a locked issue, so the reason
		// cannot change; gh fails the same way without a prompt.
		return fmt.Errorf("issue %s (%s) is already locked; unlock it first to lock it with another reason", opts.issue(), state.Title)
	}

	var body struct {
		Reason string `json:"lock_reason,omitempty"`
	}
	body.Reason = reasons[opts.Reason]
	if err := setLocked(opts, "PUT", body); err != nil {
		return err
	}
	as := ""
	if opts.Reason != "" {
		as = " as " + strings.ToUpper(opts.Reason)
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Locked%s: Issue %s (%s)\n", as, opts.issue(), state.Title)
	return nil
}

// UnlockRun unlocks the issue with DELETE /repos/{owner}/{repo}/issues/{index}/lock.
func UnlockRun(opts *LockOptions) error {
	state, err := fetchLockState(opts)
	if err != nil {
		return err
	}
	if !state.Locked {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Issue %s (%s) is not locked\n", opts.issue(), state.Title)
		return nil
	}

	if err := setLocked(opts, "DELETE", nil); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Unlocked: Issue %s (%s)\n", opts.issue(), state.Title)
	return nil
}

type lockState struct {
	Title  string `json:"title"`
	Locked bool   `json:"is_locked"`
}

// fetchLockState reads the issue with GET /repos/{owner}/{repo}/issues/{index}.
func fetchLockState(opts *LockOptions) (*lockState, error) {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d", opts.Host, opts.Owner, opts.Repo, opts.Number)
	resp, err := request(opts, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("issue %s not found", opts.issue())
	default:
		return nil, fmt.Errorf("failed to read issue %s (HTTP %d)", opts.issue(), resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var state lockState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// setLocked sends method to /repos/{owner}/{repo}/issues/{index}/lock: PUT
// locks, DELETE unlocks. Call it after fetchLockState: a 404 then means the
// server does not serve the route.
func setLocked(opts *LockOptions, method string, body any) error {
	verb := "unlock"
	if method == "PUT" {
		verb = "lock"
	}
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/lock", opts.Host, opts.Owner, opts.Repo, opts.Number)
	resp, err := request(opts, method, url, payload)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("cannot %s issue %s: permission denied (HTTP 403); %sing an issue needs write access to the repository's issues", verb, opts.issue(), verb)
	case http.StatusNotFound:
		// The issue was just read, so the server does not serve the route.
		return fmt.Errorf("%s does not support %sing issues (HTTP 404 on %s /repos/%s/%s/issues/%d/lock)",
			opts.Host, verb, method, opts.Owner, opts.Repo, opts.Number)
	}

	var apiErr struct {
		Message string `json:"message"`
	}
	if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("failed to %s issue %s (HTTP %d): %s", verb, opts.issue(), resp.StatusCode, apiErr.Message)
	}
	return fmt.Errorf("failed to %s issue %s (HTTP %d)", verb, opts.issue(), resp.StatusCode)
}

func request(opts *LockOptions, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return opts.HTTPClient.Do(req)
}
