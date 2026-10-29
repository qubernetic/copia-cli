package close

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CloseOptions holds all inputs for the pr close command.
type CloseOptions struct {
	IO           *iostreams.IOStreams
	HTTPClient   *http.Client
	Host         string
	Token        string
	Owner        string
	Repo         string
	Number       int64
	Comment      string
	DeleteBranch bool
}

// NewCmdClose creates the `copia pr close` command.
func NewCmdClose(f *cmdutil.Factory) *cobra.Command {
	opts := &CloseOptions{}

	cmd := &cobra.Command{
		Use:     "close <number>",
		Short:   "Close a pull request",
		Long:    "Close a pull request by number. Optionally add a closing comment with --comment or delete the branch with --delete-branch.",
		Example: `  # Close a pull request
  $ copia-cli pr close 7

  # Close with a comment
  $ copia-cli pr close 7 --comment "Closing in favor of #8"

  # Close and delete the branch
  $ copia-cli pr close 7 --delete-branch`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			num, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid PR number: %s", args[0])
			}
			opts.Number = num
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return err
			}
			opts.Owner = owner
			opts.Repo = repo
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return CloseRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Leave a closing comment")
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the local and remote branch after close")

	return cmd
}

func CloseRun(opts *CloseOptions) error {
	// Add comment before closing if requested
	if opts.Comment != "" {
		commentPayload, _ := json.Marshal(map[string]string{"body": opts.Comment})
		commentURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/comments",
			opts.Host, opts.Owner, opts.Repo, opts.Number)

		req, err := http.NewRequest("POST", commentURL, bytes.NewReader(commentPayload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "token "+opts.Token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := opts.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to add comment (HTTP %d)", resp.StatusCode)
		}
	}

	// Close the PR
	payload, _ := json.Marshal(map[string]string{"state": "closed"})
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to close PR (HTTP %d)", resp.StatusCode)
	}

	// Read response to get head branch for delete
	var prData struct {
		Head struct {
			Label string `json:"label"`
		} `json:"head"`
	}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &prData)

	_, _ = fmt.Fprintf(opts.IO.Out, "Closed PR #%d\n", opts.Number)

	// Delete branch if requested
	if opts.DeleteBranch && prData.Head.Label != "" {
		branchURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/branches/%s",
			opts.Host, opts.Owner, opts.Repo, prData.Head.Label)

		delReq, err := http.NewRequest("DELETE", branchURL, nil)
		if err != nil {
			return err
		}
		delReq.Header.Set("Authorization", "token "+opts.Token)

		delResp, err := opts.HTTPClient.Do(delReq)
		if err != nil {
			return fmt.Errorf("failed to delete branch: %w", err)
		}
		defer func() { _ = delResp.Body.Close() }()

		if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusOK {
			_, _ = fmt.Fprintf(opts.IO.Out, "Deleted branch %s\n", prData.Head.Label)
		}
	}

	return nil
}
