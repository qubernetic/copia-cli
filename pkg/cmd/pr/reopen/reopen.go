package reopen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/pr/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ReopenOptions holds all inputs for the pr reopen command.
type ReopenOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Comment    string
}

// NewCmdReopen creates the `copia pr reopen` command.
func NewCmdReopen(f *cmdutil.Factory) *cobra.Command {
	opts := &ReopenOptions{}

	cmd := &cobra.Command{
		Use:   "reopen {<number> | <url>}",
		Short: "Reopen a pull request",
		Long: "Reopen a closed pull request by number or URL. Optionally add a comment with --comment.\n\n" +
			"A pull request that is already open is left as it is, and no comment is added. " +
			"A merged pull request cannot be reopened.",
		Example: `  # Reopen a pull request
  $ copia-cli pr reopen 7

  # Reopen by URL with a comment
  $ copia-cli pr reopen https://app.copia.io/my-org/my-repo/pulls/7 --comment "Back on track"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := shared.ParsePRArg(args[0])
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
			return ReopenRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Add a reopening comment")

	return cmd
}

type prState struct {
	Title  string `json:"title"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
}

// ReopenRun reopens the pull request with PATCH /repos/{owner}/{repo}/pulls/{index}.
func ReopenRun(opts *ReopenOptions) error {
	prURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)
	ref := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)

	pr, err := fetchPR(opts, prURL, ref)
	if err != nil {
		return err
	}
	if pr.Merged {
		return fmt.Errorf("pull request %s (%s) can't be reopened because it was already merged", ref, pr.Title)
	}
	if pr.State == "open" {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Pull request %s (%s) is already open\n", ref, pr.Title)
		return nil
	}

	if opts.Comment != "" {
		// Gitea keeps pull request comments on the issue of the same index.
		commentURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/comments",
			opts.Host, opts.Owner, opts.Repo, opts.Number)
		commentPayload, _ := json.Marshal(map[string]string{"body": opts.Comment})
		resp, err := send(opts, "POST", commentURL, commentPayload)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to add comment (HTTP %d)", resp.StatusCode)
		}
	}

	reopenPayload, _ := json.Marshal(map[string]string{"state": "open"})
	resp, err := send(opts, "PATCH", prURL, reopenPayload)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var apiErr struct {
			Message string `json:"message"`
		}
		if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return fmt.Errorf("failed to reopen pull request %s (HTTP %d): %s", ref, resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("failed to reopen pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Reopened pull request %s (%s)\n", ref, pr.Title)
	return nil
}

// fetchPR reads the pull request with GET /repos/{owner}/{repo}/pulls/{index}.
func fetchPR(opts *ReopenOptions, prURL, ref string) (*prState, error) {
	resp, err := send(opts, "GET", prURL, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("pull request %s not found", ref)
	default:
		return nil, fmt.Errorf("failed to read pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pr prState
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func send(opts *ReopenOptions, method, url string, payload []byte) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return opts.HTTPClient.Do(req)
}
