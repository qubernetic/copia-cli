package pin

import (
	"fmt"
	"net/http"

	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// PinOptions holds all inputs for the issue pin command.
type PinOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
}

// NewCmdPin creates the `copia issue pin` command.
func NewCmdPin(f *cmdutil.Factory) *cobra.Command {
	opts := &PinOptions{}

	cmd := &cobra.Command{
		Use:   "pin {<number> | <url>}",
		Short: "Pin an issue",
		Long: "Pin an issue to the repository by number or URL.\n\n" +
			"An issue that is already pinned is left as it is. Pinning needs write access to the\n" +
			"repository's issues.",
		Example: `  # Pin an issue to the current repository
  $ copia-cli issue pin 23

  # Pin an issue by URL
  $ copia-cli issue pin https://app.copia.io/my-org/my-repo/issues/23

  # Pin an issue to a specific repository
  $ copia-cli issue pin 23 --repo my-org/my-repo`,
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
			return PinRun(opts)
		},
	}

	return cmd
}

func PinRun(opts *PinOptions) error {
	state, err := shared.FetchPinState(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Number)
	if err != nil {
		return err
	}
	issue := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)
	if pinned, _ := state.Pinned(); pinned {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Issue %s (%s) is already pinned\n", issue, state.Title)
		return nil
	}

	if err := shared.SetPinned(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Number, true); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Pinned issue %s (%s)\n", issue, state.Title)
	return nil
}
