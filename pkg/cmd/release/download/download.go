package download

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/release/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// DownloadOptions holds all inputs for the release download command.
type DownloadOptions struct {
	IO                *iostreams.IOStreams
	HTTPClient        *http.Client
	Host              string
	Token             string
	Owner             string
	Repo              string
	Tag               string
	FilePatterns      []string
	ArchiveType       string
	Destination       string
	OutputFile        string
	OverwriteExisting bool
	SkipExisting      bool
}

// NewCmdDownload creates the `copia-cli release download` command.
func NewCmdDownload(f *cmdutil.Factory) *cobra.Command {
	opts := &DownloadOptions{}

	cmd := &cobra.Command{
		Use:   "download [<tag>]",
		Short: "Download release assets",
		Long: `Download assets from a Copia release.

Without an explicit tag name argument, assets are downloaded from the
latest release in the project. In this case, ` + "`--pattern` or `--archive`" + `
is required.`,
		Example: `  # Download all assets from a specific release
  $ copia-cli release download v1.2.3

  # Download only Debian packages for the latest release
  $ copia-cli release download --pattern '*.deb'

  # Specify multiple file patterns
  $ copia-cli release download -p '*.deb' -p '*.rpm'

  # Download the archive of the source code for a release
  $ copia-cli release download v1.2.3 --archive=zip`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams

			if len(args) == 0 {
				if len(opts.FilePatterns) == 0 && opts.ArchiveType == "" {
					return errors.New("`--pattern` or `--archive` is required when downloading the latest release")
				}
			} else {
				opts.Tag = args[0]
			}
			if opts.OverwriteExisting && opts.SkipExisting {
				return errors.New("specify only one of `--clobber` or `--skip-existing`")
			}
			if opts.Destination != "." && opts.OutputFile != "" {
				return errors.New("specify only one of `--dir` or `--output`")
			}
			if opts.ArchiveType != "" {
				if len(opts.FilePatterns) > 0 {
					return errors.New("specify only one of '--pattern' or '--archive'")
				}
				if opts.ArchiveType != "zip" && opts.ArchiveType != "tar.gz" {
					return errors.New("the value for `--archive` must be one of \"zip\" or \"tar.gz\"")
				}
			}

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
			return DownloadRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFile, "output", "O", "", "The `file` to write a single asset to (use \"-\" to write to standard output)")
	cmd.Flags().StringVarP(&opts.Destination, "dir", "D", ".", "The `directory` to download files into")
	cmd.Flags().StringArrayVarP(&opts.FilePatterns, "pattern", "p", nil, "Download only assets that match a glob pattern")
	cmd.Flags().StringVarP(&opts.ArchiveType, "archive", "A", "", "Download the source code archive in the specified `format` (zip or tar.gz)")
	cmd.Flags().BoolVar(&opts.OverwriteExisting, "clobber", false, "Overwrite existing files of the same name")
	cmd.Flags().BoolVar(&opts.SkipExisting, "skip-existing", false, "Skip downloading when files of the same name exist")

	return cmd
}

// target is one file to download: an asset, or the source archive, whose
// name comes from the response when it is empty.
type target struct {
	url      string
	name     string
	archive  bool
	fallback string
}

var errSkipped = errors.New("skipped")

// DownloadRun downloads the matching assets of the release Tag (the latest
// release when Tag is empty), or its source archive, into Destination or
// OutputFile. Every request goes to the configured host only.
func DownloadRun(opts *DownloadOptions) error {
	hostURL, err := cmdutil.ParseHost(opts.Host)
	if err != nil {
		return err
	}

	release, err := shared.FetchRelease(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Tag)
	if err != nil {
		return err
	}

	var targets []target
	if opts.ArchiveType != "" {
		if release.TagName == "" {
			return fmt.Errorf("release %q has no tag, so it has no %q archive", release.Name, opts.ArchiveType)
		}
		targets = append(targets, target{
			url: fmt.Sprintf("https://%s/api/v1/repos/%s/%s/archive/%s.%s",
				opts.Host, opts.Owner, opts.Repo, escapeRef(release.TagName), opts.ArchiveType),
			archive:  true,
			fallback: opts.Repo + "-" + release.TagName + "." + opts.ArchiveType,
		})
	} else {
		for _, a := range release.Assets {
			if len(opts.FilePatterns) > 0 && !matchAny(opts.FilePatterns, a.Name) {
				continue
			}
			targets = append(targets, target{url: a.BrowserDownloadURL, name: a.Name})
		}
	}

	if len(targets) == 0 {
		if len(release.Assets) > 0 {
			return errors.New("no assets match the file pattern")
		}
		return errors.New("no assets to download")
	}
	if len(targets) > 1 && opts.OutputFile != "" {
		return fmt.Errorf("unable to write more than one asset with `--output`, got %d assets", len(targets))
	}

	dest := &destination{
		file:      opts.OutputFile,
		dir:       opts.Destination,
		skip:      opts.SkipExisting,
		overwrite: opts.OverwriteExisting,
		stdout:    opts.IO.Out,
	}
	for _, t := range targets {
		if err := download(opts, hostURL, dest, t); err != nil && !errors.Is(err, errSkipped) {
			return err
		}
	}
	return nil
}

// escapeRef path-escapes every segment of a tag, keeping its slashes.
func escapeRef(ref string) string {
	segments := strings.Split(ref, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

func download(opts *DownloadOptions, hostURL *url.URL, dest *destination, t target) error {
	if !t.archive {
		if err := dest.check(t.name); err != nil {
			return err
		}
	}
	// The download URL of an asset comes from the server. The token goes to
	// the configured host only, so any other URL is refused unrequested.
	if !cmdutil.MatchesHost(hostURL, t.url) {
		return fmt.Errorf("refusing to download %s: its URL is not on %s", displayName(t), opts.Host)
	}

	req, err := http.NewRequest("GET", t.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		// A url.Error names the last URL, which after a redirect can be a
		// signed storage URL; its cause names the hosts only.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("downloading %s: %w", displayName(t), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s (HTTP %d)", displayName(t), resp.StatusCode)
	}

	name := t.name
	if t.archive {
		name = t.fallback
		if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
			name = params["filename"]
		}
	}
	return dest.copy(name, resp.Body)
}

func displayName(t target) string {
	if t.archive {
		return "the source archive"
	}
	return t.name
}

// destination writes downloaded content to a directory, a single file or
// standard output.
type destination struct {
	file      string
	dir       string
	skip      bool
	overwrite bool
	stdout    io.Writer
}

func (d *destination) path(name string) string {
	if d.file != "" {
		return d.file
	}
	return filepath.Join(d.dir, name)
}

// check validates name and reports whether its file may be written:
// errSkipped with --skip-existing, an error without --clobber.
func (d *destination) check(name string) error {
	if d.file == "-" {
		return nil
	}
	if d.file == "" {
		if err := validateName(name); err != nil {
			return err
		}
	}
	fp := d.path(name)
	_, err := os.Lstat(fp)
	switch {
	case err == nil && d.skip:
		return errSkipped
	case err == nil && !d.overwrite:
		return fmt.Errorf("%s already exists (use `--clobber` to overwrite file or `--skip-existing` to skip file)", fp)
	case err != nil && !os.IsNotExist(err):
		return err
	}
	return nil
}

// validateName accepts a plain file name only, so nothing is written outside
// the target directory.
func validateName(name string) error {
	if name == "" || name == "." || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) || !filepath.IsLocal(name) {
		return fmt.Errorf("refusing to write %q: an asset name must be a plain file name", name)
	}
	return nil
}

// copy writes r to the file of name through a temporary file in the same
// directory, renamed into place once complete, or to standard output.
func (d *destination) copy(name string, r io.Reader) error {
	if d.file == "-" {
		_, err := io.Copy(d.stdout, r)
		return err
	}
	if err := d.check(name); err != nil {
		return err
	}
	fp := d.path(name)
	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".copia-download-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	// Check again: the file may have appeared during the download.
	if err := d.check(name); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), fp)
}
