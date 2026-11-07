package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// SyncOptions holds all inputs for the repo sync command.
type SyncOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	// BaseRepo resolves the default repository of the local mode
	// (-R, the remote marked by repo set-default, then origin).
	BaseRepo func() (string, string, error)
	DestArg  string
	SrcArg   string
	Branch   string
	Force    bool
}

// NewCmdSync creates the `copia repo sync` command.
func NewCmdSync(f *cmdutil.Factory) *cobra.Command {
	opts := &SyncOptions{}

	cmd := &cobra.Command{
		Use:   "sync [<destination-repository>]",
		Short: "Sync a repository",
		Long: `Sync destination repository from source repository. Syncing uses the default branch
of the source repository to update the matching branch on the destination
repository so they are equal. A fast forward update will be used except when the
--force flag is specified, then the two branches will
be synced using a hard reset.

Without an argument, the local repository is selected as the destination repository.
The source repository is --source, or else the default repository (the remote marked
by repo set-default, else origin), and must be one of the git remotes.

With an argument, the remote repository, a fork, is synced from its parent on the
server, fast-forward only. Copia (app.copia.io) does not offer fork sync through its
API, so there the remote mode fails with an error; it works against a Gitea-compatible
server that serves the merge-upstream endpoint.`,
		Example: `  # Sync local repository from the default repository
  $ copia-cli repo sync

  # Sync local repository from the default repository on a specific branch
  $ copia-cli repo sync --branch v1

  # Sync local repository from another remote, overwriting local commits
  $ copia-cli repo sync --source upstream-org/project --force

  # Sync remote fork from its parent
  $ copia-cli repo sync owner/project-fork`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if len(args) > 0 {
				opts.DestArg = args[0]
			}
			opts.BaseRepo = f.ResolveRepo

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return SyncRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.SrcArg, "source", "s", "", "Source repository")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Branch to sync (default [default branch])")
	cmd.Flags().BoolVarP(&opts.Force, "force", "", false, "Hard reset the branch of the destination repository to match the source repository")

	return cmd
}

// SyncRun syncs the local repository without a destination argument, the
// remote repository named by it otherwise.
func SyncRun(opts *SyncOptions) error {
	if opts.DestArg == "" {
		return syncLocalRepo(opts)
	}
	return syncRemoteRepo(opts)
}

// syncRemoteRepo brings a branch of the fork DestArg up to date with its
// parent with POST /repos/{owner}/{repo}/merge-upstream, fast-forward only.
func syncRemoteRepo(opts *SyncOptions) error {
	if opts.Force {
		return errors.New("--force is not supported with a repository argument; run copia-cli repo sync --force in a local clone and push")
	}
	if opts.SrcArg != "" {
		return errors.New("--source is not supported with a repository argument; a remote repository syncs from its parent")
	}
	owner, repo, err := cmdutil.SplitOwnerRepo(opts.DestArg)
	if err != nil {
		return err
	}

	branch := opts.Branch
	if branch == "" {
		if branch, err = defaultBranch(opts, owner, repo); err != nil {
			return err
		}
	}

	body, err := json.Marshal(map[string]any{"branch": branch, "ff_only": true})
	if err != nil {
		return err
	}
	resp, err := apiRequest(opts, "POST", fmt.Sprintf("/repos/%s/%s/merge-upstream", owner, repo), body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if isUnroutedNotFound(resp) {
		return errors.New("Copia does not offer fork sync through its API") //nolint:staticcheck // Copia is a proper name
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		message := apiMessage(data)
		switch {
		case resp.StatusCode == http.StatusBadRequest && strings.Contains(message, "not a fork"):
			return fmt.Errorf("%s/%s is not a fork", owner, repo)
		case resp.StatusCode == http.StatusConflict,
			resp.StatusCode == http.StatusBadRequest && strings.Contains(message, "diverged"):
			return errors.New("can't sync because there are diverging changes; run copia-cli repo sync --force in a local clone and push")
		}
		return apiError(fmt.Sprintf("failed to sync %s/%s", owner, repo), resp.StatusCode, message)
	}

	var result struct {
		MergeType string `json:"merge_type"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to decode the merge-upstream response: %w", err)
	}
	if result.MergeType == "up-to-date" {
		_, _ = fmt.Fprintf(opts.IO.Out, "The \"%s:%s\" branch is already up to date with its parent\n", owner, branch)
		return nil
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Synced the \"%s:%s\" branch from its parent\n", owner, branch)
	return nil
}

// defaultBranch returns the default branch of owner/repo from
// GET /repos/{owner}/{repo}.
func defaultBranch(opts *SyncOptions, owner, repo string) (string, error) {
	resp, err := apiRequest(opts, "GET", fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", apiError(fmt.Sprintf("failed to get repository %s/%s", owner, repo), resp.StatusCode, apiMessage(data))
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("failed to decode repository %s/%s: %w", owner, repo, err)
	}
	if info.DefaultBranch == "" {
		return "", fmt.Errorf("repository %s/%s has no default branch", owner, repo)
	}
	return info.DefaultBranch, nil
}

// apiRequest sends method to the API path of the configured host with the
// token and, when body is not nil, a JSON body.
func apiRequest(opts *SyncOptions, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("https://%s/api/v1%s", opts.Host, path), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	return resp, nil
}

// apiMessage returns the message of a Gitea JSON error body, or "".
func apiMessage(data []byte) string {
	var apiErr struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &apiErr) != nil {
		return ""
	}
	return apiErr.Message
}

func apiError(prefix string, status int, message string) error {
	if message == "" {
		return fmt.Errorf("%s (HTTP %d)", prefix, status)
	}
	return fmt.Errorf("%s (HTTP %d): %s", prefix, status, message)
}

// isUnroutedNotFound reports a plain-text 404, the answer of a server without
// the merge-upstream route (app.copia.io has none). Gitea answers a missing
// repository with a JSON 404.
func isUnroutedNotFound(resp *http.Response) bool {
	if resp.StatusCode != http.StatusNotFound {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return err == nil && mediaType == "text/plain"
}

func syncLocalRepo(opts *SyncOptions) error {
	return errors.New("not implemented")
}
