package comment

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

// CommentOptions holds all inputs for the issue comment command.
type CommentOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Body       string
}

// NewCmdComment creates the `copia issue comment` command.
func NewCmdComment(f *cmdutil.Factory) *cobra.Command {
	opts := &CommentOptions{}

	cmd := &cobra.Command{
		Use:     "comment <number>",
		Short:   "Add a comment to an issue",
		Long:    "Add a comment to a Copia issue. The comment body is supplied via the --body flag.",
		Example: `  $ copia-cli issue comment 12 --body "Investigating this now."`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			num, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid issue number: %s", args[0])
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
			return CommentRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Comment body (required)")

	return cmd
}

func CommentRun(opts *CommentOptions) error {
	if opts.Body == "" {
		return fmt.Errorf("body required")
	}

	payload, _ := json.Marshal(map[string]string{"body": opts.Body})
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/comments",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
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

	_, _ = fmt.Fprintf(opts.IO.Out, "Comment added to issue #%d\n", opts.Number)
	return nil
}
