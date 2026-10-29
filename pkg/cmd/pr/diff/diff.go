package diff

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type DiffOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
}

func NewCmdDiff(f *cmdutil.Factory) *cobra.Command {
	opts := &DiffOptions{}

	cmd := &cobra.Command{
		Use:     "diff <number>",
		Short:   "View diff for a pull request",
		Long:    "View changes in a pull request as a unified diff.",
		Example: "  $ copia-cli pr diff 7",
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
			return DiffRun(opts)
		},
	}

	return cmd
}

func DiffRun(opts *DiffOptions) error {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d.diff",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("PR #%d not found", opts.Number)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	_, err = io.Copy(opts.IO.Out, resp.Body)
	return err
}
