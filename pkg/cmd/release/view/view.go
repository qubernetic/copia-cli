package view

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/release/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ViewOptions holds all inputs for the release view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Tag        string
	JSON       cmdutil.JSONFlags
	Web        bool
	// Now is the clock the "released this" line measures against.
	Now func() time.Time
}

// NewCmdView creates the `copia-cli release view` command.
func NewCmdView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{}

	cmd := &cobra.Command{
		Use:   "view [<tag>]",
		Short: "View information about a release",
		Long: `View information about a Copia release.

Without an explicit tag name argument, the latest release in the project
is shown: the newest release that is neither a draft nor a pre-release.

With --web, open the release page in the browser instead. With a tag no API
call is made, so no token is needed; without one the latest release is
looked up first.`,
		Example: `  # View the latest release
  $ copia-cli release view

  # View a release by tag
  $ copia-cli release view v1.0.0

  # List the asset names as JSON
  $ copia-cli release view v1.0.0 --json assets --jq '.assets[].name'

  # Open the release in the browser
  $ copia-cli release view v1.0.0 --web`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Tag = args[0]
			}

			if opts.Web {
				return openRelease(f, opts.Tag)
			}

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
			opts.Now = time.Now
			return ViewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the release in the browser")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, shared.ReleaseFields)

	return cmd
}

// openRelease opens the web page of the release with tag without an API
// call. Without a tag it looks the latest release up and opens its page, as
// gh does, so that needs a token.
func openRelease(f *cmdutil.Factory, tag string) error {
	if tag == "" {
		return openLatestRelease(f)
	}
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}
	owner, repo, err := f.ResolveRepo()
	if err != nil {
		return err
	}
	return cmdutil.OpenInBrowser(f.IOStreams, f.Browser, cmdutil.RepoWebURL(host, owner, repo, "releases/tag/"+url.PathEscape(tag)))
}

func openLatestRelease(f *cmdutil.Factory) error {
	host, token, err := f.ResolveAuth()
	if err != nil {
		return err
	}
	owner, repo, err := f.ResolveRepo()
	if err != nil {
		return err
	}
	release, err := shared.FetchRelease(cmdutil.NewHTTPClient(), host, token, owner, repo, "")
	if err != nil {
		return err
	}
	return cmdutil.OpenInBrowser(f.IOStreams, f.Browser, release.HTMLURL)
}

func ViewRun(opts *ViewOptions) error {
	release, err := shared.FetchRelease(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Tag)
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, release)
	}

	if opts.IO.IsStdoutTTY() {
		return renderReleaseTTY(opts.IO.Out, release, opts.Now())
	}
	renderReleasePlain(opts.IO.Out, release)
	return nil
}

// renderReleaseTTY prints the release as gh does on a terminal: the tag, who
// released it how long ago, the notes, a table of assets and the web URL.
func renderReleaseTTY(w io.Writer, release *shared.Release, now time.Time) error {
	_, _ = fmt.Fprintf(w, "%s\n", release.TagName)
	if release.Draft {
		_, _ = fmt.Fprint(w, "Draft • ")
	} else if release.Prerelease {
		_, _ = fmt.Fprint(w, "Pre-release • ")
	}
	if release.Draft {
		_, _ = fmt.Fprintf(w, "%s created this %s\n", release.Author.Login, timeAgo(now, release.CreatedAt))
	} else {
		_, _ = fmt.Fprintf(w, "%s released this %s\n", release.Author.Login, timeAgo(now, release.PublishedAt))
	}
	_, _ = fmt.Fprintln(w)

	if release.Body != "" {
		_, _ = fmt.Fprintf(w, "%s\n\n", strings.TrimRight(release.Body, "\n"))
	}

	if len(release.Assets) > 0 {
		_, _ = fmt.Fprintln(w, "Assets")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tSIZE")
		for _, a := range release.Assets {
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", a.Name, humanFileSize(a.Size))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintf(w, "View on Copia: %s\n", release.HTMLURL)
	return nil
}

// renderReleasePlain prints the release as gh does without a terminal:
// tab-separated header lines, "--", then the notes.
func renderReleasePlain(w io.Writer, release *shared.Release) {
	_, _ = fmt.Fprintf(w, "title:\t%s\n", release.Name)
	_, _ = fmt.Fprintf(w, "tag:\t%s\n", release.TagName)
	_, _ = fmt.Fprintf(w, "draft:\t%v\n", release.Draft)
	_, _ = fmt.Fprintf(w, "prerelease:\t%v\n", release.Prerelease)
	_, _ = fmt.Fprintf(w, "author:\t%s\n", release.Author.Login)
	_, _ = fmt.Fprintf(w, "created:\t%s\n", release.CreatedAt)
	if !release.Draft {
		_, _ = fmt.Fprintf(w, "published:\t%s\n", release.PublishedAt)
	}
	_, _ = fmt.Fprintf(w, "url:\t%s\n", release.HTMLURL)
	for _, a := range release.Assets {
		_, _ = fmt.Fprintf(w, "asset:\t%s\n", a.Name)
	}
	_, _ = fmt.Fprint(w, "--\n")
	_, _ = fmt.Fprint(w, release.Body)
	if !strings.HasSuffix(release.Body, "\n") {
		_, _ = fmt.Fprint(w, "\n")
	}
}

// timeAgo describes an RFC 3339 time in words, or returns it unchanged when
// it does not parse.
func timeAgo(now time.Time, s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return cmdutil.TimeAgo(now, t)
}

// humanFileSize formats a byte count as gh does: whole bytes below 1 KiB,
// otherwise KiB, MiB or GiB truncated to two decimals.
func humanFileSize(s int64) string {
	if s < 1024 {
		return fmt.Sprintf("%d B", s)
	}
	kb := float64(s) / 1024
	if kb < 1024 {
		return truncate2(kb) + " KiB"
	}
	mb := kb / 1024
	if mb < 1024 {
		return truncate2(mb) + " MiB"
	}
	return truncate2(mb/1024) + " GiB"
}

func truncate2(f float64) string {
	fs := fmt.Sprintf("%f", f)
	return fs[:strings.IndexByte(fs, '.')+3]
}
