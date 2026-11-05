package reopen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ReopenOptions holds all inputs for the issue reopen command.
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

// NewCmdReopen creates the `copia issue reopen` command.
func NewCmdReopen(f *cmdutil.Factory) *cobra.Command {
	opts := &ReopenOptions{}

	cmd := &cobra.Command{
		Use:   "reopen {<number> | <url>}",
		Short: "Reopen an issue",
		Long: "Reopen a closed issue by number or URL. Optionally add a comment with --comment.\n\n" +
			"An issue that is already open is left as it is, and no comment is added.",
		Example: `  # Reopen an issue
  $ copia-cli issue reopen 12

  # Reopen by URL with a comment
  $ copia-cli issue reopen https://app.copia.io/my-org/my-repo/issues/12 --comment "Still failing"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := shared.ParseIssueArg(args[0])
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

type issueState struct {
	Number int64  `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

func ReopenRun(opts *ReopenOptions) error {
	issueURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	issue, err := fetchIssue(opts, issueURL)
	if err != nil {
		return err
	}
	if issue.State == "open" {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Issue %s/%s#%d (%s) is already open\n",
			opts.Owner, opts.Repo, opts.Number, issue.Title)
		return nil
	}

	if opts.Comment != "" {
		commentPayload, _ := json.Marshal(map[string]string{"body": opts.Comment})
		resp, err := send(opts, "POST", issueURL+"/comments", commentPayload)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to add comment (HTTP %d)", resp.StatusCode)
		}
	}

	reopenPayload, _ := json.Marshal(map[string]string{"state": "open"})
	resp, err := send(opts, "PATCH", issueURL, reopenPayload)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to reopen issue (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Reopened issue #%d\n", opts.Number)
	return nil
}

func fetchIssue(opts *ReopenOptions, issueURL string) (*issueState, error) {
	resp, err := send(opts, "GET", issueURL, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("issue #%d not found", opts.Number)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read issue (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var issue issueState
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
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
