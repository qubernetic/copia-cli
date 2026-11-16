package unpin

import (
	"fmt"
	"net/http"

	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// UnpinOptions holds all inputs for the issue unpin command.
type UnpinOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
}

// NewCmdUnpin creates the `copia issue unpin` command.
func NewCmdUnpin(f *cmdutil.Factory) *cobra.Command {
	opts := &UnpinOptions{}

	cmd := &cobra.Command{
		Use:   "unpin {<number> | <url>}",
		Short: "Unpin an issue",
		Long: "Unpin an issue from the repository by number or URL.\n\n" +
			"An issue that is not pinned is left as it is. Unpinning needs write access to the\n" +
			"repository's issues.\n\n" +
			"app.copia.io does not serve the pin endpoint and answers with HTTP 404, so the command\n" +
			"reports that the host does not support it.",
		Example: `  # Unpin an issue from the current repository
  $ copia-cli issue unpin 23

  # Unpin an issue by URL
  $ copia-cli issue unpin https://app.copia.io/my-org/my-repo/issues/23

  # Unpin an issue from a specific repository
  $ copia-cli issue unpin 23 --repo my-org/my-repo`,
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
			return UnpinRun(opts)
		},
	}

	return cmd
}

func UnpinRun(opts *UnpinOptions) error {
	state, err := shared.FetchPinState(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Number)
	if err != nil {
		return err
	}
	issue := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)
	if pinned, known := state.Pinned(); known && !pinned {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Issue %s (%s) is not pinned\n", issue, state.Title)
		return nil
	}

	if err := shared.SetPinned(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Number, false); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Unpinned issue %s (%s)\n", issue, state.Title)
	return nil
}
