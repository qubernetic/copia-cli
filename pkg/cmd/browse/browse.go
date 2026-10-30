// Package browse implements the `copia-cli browse` command.
package browse

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// BrowseOptions holds all inputs for the browse command.
type BrowseOptions struct {
	IO        *iostreams.IOStreams
	Browser   cmdutil.Browser
	Host      string
	Owner     string
	Repo      string
	Number    int
	NoBrowser bool
}

// NewCmdBrowse creates the `copia-cli browse` command.
func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {
	opts := &BrowseOptions{}

	cmd := &cobra.Command{
		Use:   "browse [<number>]",
		Short: "Open a repository, issue or pull request in the browser",
		Long: `Open the repository's web page in the browser. With a number, open that issue or
pull request instead; Copia redirects an issue URL to the pull request with the
same number.

No API call is made, so no token is needed. The browser is the command in
COPIA_BROWSER or BROWSER, otherwise the system's default opener.`,
		Example: `  # Open the current repository
  $ copia-cli browse

  # Open issue or pull request 42
  $ copia-cli browse 42

  # Print the URL of a pull request in another repository
  $ copia-cli browse 7 -R my-org/my-repo --no-browser`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Browser = f.Browser

			if len(args) > 0 {
				n, err := strconv.Atoi(strings.TrimPrefix(args[0], "#"))
				if err != nil || n < 1 {
					return fmt.Errorf("invalid argument %q: expected an issue or pull request number", args[0])
				}
				opts.Number = n
			}

			host, err := f.ResolveHost()
			if err != nil {
				return err
			}
			opts.Host = host

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return fmt.Errorf("could not determine repository: %w", err)
			}
			opts.Owner, opts.Repo = owner, repo

			return BrowseRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.NoBrowser, "no-browser", "n", false, "Print destination URL instead of opening the browser")

	return cmd
}

// BrowseRun opens, or with NoBrowser prints, the URL of the repository or
// of the issue or pull request opts.Number.
func BrowseRun(opts *BrowseOptions) error {
	u := fmt.Sprintf("https://%s/%s/%s", opts.Host, url.PathEscape(opts.Owner), url.PathEscape(opts.Repo))
	if opts.Number > 0 {
		u += fmt.Sprintf("/issues/%d", opts.Number)
	}

	if opts.NoBrowser {
		_, err := fmt.Fprintln(opts.IO.Out, u)
		return err
	}

	if opts.IO.IsStdoutTTY() {
		_, _ = fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", strings.TrimPrefix(u, "https://"))
	}
	return opts.Browser.Browse(u)
}
