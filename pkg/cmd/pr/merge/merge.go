package merge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// MergeOptions holds all inputs for the pr merge command.
type MergeOptions struct {
	IO           *iostreams.IOStreams
	HTTPClient   *http.Client
	Host         string
	Token        string
	Owner        string
	Repo         string
	Number       int64
	Method       string
	DeleteBranch bool
	Body         string
}

type mergeRequest struct {
	Do                     string `json:"Do"`
	MergeMessageField      string `json:"MergeMessageField,omitempty"`
	DeleteBranchAfterMerge bool   `json:"delete_branch_after_merge,omitempty"`
}

// NewCmdMerge creates the `copia pr merge` command.
func NewCmdMerge(f *cmdutil.Factory) *cobra.Command {
	opts := &MergeOptions{}

	cmd := &cobra.Command{
		Use:   "merge <number>",
		Short: "Merge a pull request",
		Long:  "Merge a pull request on Copia. By default a merge commit is created. Use --squash or --rebase to change the merge method.",
		Example: `  # Merge a pull request
  $ copia-cli pr merge 7

  # Squash and merge
  $ copia-cli pr merge 7 --squash

  # Rebase and delete the branch
  $ copia-cli pr merge 7 --rebase --delete-branch

  # Merge with a body for the merge commit
  $ copia-cli pr merge 7 --squash --body "Tested on the line 3 PLC"`,
		Args: cobra.ExactArgs(1),
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
			opts.HTTPClient = &http.Client{}

			// Determine merge method from flags
			squash, _ := cmd.Flags().GetBool("squash")
			rebase, _ := cmd.Flags().GetBool("rebase")
			if squash {
				opts.Method = "squash"
			} else if rebase {
				opts.Method = "rebase"
			} else {
				opts.Method = "merge"
			}

			return MergeRun(opts)
		},
	}

	cmd.Flags().Bool("merge", false, "Merge commit (default)")
	cmd.Flags().Bool("squash", false, "Squash and merge")
	cmd.Flags().Bool("rebase", false, "Rebase and merge")
	cmd.Flags().BoolVar(&opts.DeleteBranch, "delete-branch", false, "Delete branch after merge")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body text for the merge commit")

	return cmd
}

func MergeRun(opts *MergeOptions) error {
	payload := mergeRequest{
		Do:                     opts.Method,
		MergeMessageField:      opts.Body,
		DeleteBranchAfterMerge: opts.DeleteBranch,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d/merge",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to merge PR (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Merged PR #%d (%s)\n", opts.Number, opts.Method)
	return nil
}
