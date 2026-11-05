// Package browse implements the `copia-cli browse` command.
package browse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// lastCommit is the value of --commit given without a SHA, as in gh.
const lastCommit = "last"

// BrowseOptions holds all inputs for the browse command.
type BrowseOptions struct {
	IO         *iostreams.IOStreams
	Browser    cmdutil.Browser
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string

	SelectorArg string
	Branch      string
	Commit      string
	Projects    bool
	Releases    bool
	Settings    bool
	Wiki        bool
	Actions     bool
	Blame       bool
	NoBrowser   bool

	// HasRepoOverride is set by -R: the path argument is then taken from the
	// repository root and --commit without a SHA uses the default branch.
	HasRepoOverride bool
}

// NewCmdBrowse creates the `copia-cli browse` command.
func NewCmdBrowse(f *cmdutil.Factory) *cobra.Command {
	opts := &BrowseOptions{}

	cmd := &cobra.Command{
		Use:   "browse [<number> | <path> | <commit-sha>]",
		Short: "Open repositories, issues, pull requests, and more in the browser",
		Long: `Open the repository's web page in the browser, or with an argument:

- a number ("42" or "#42"): that issue or pull request; Copia redirects an issue
  URL to the pull request with the same number;
- a commit SHA (7 to 64 hexadecimal characters): that commit;
- a path, optionally with a line or a line range ("main.st:12", "main.st:12-20"):
  that file or directory on the default branch, or on --branch or --commit.
  A relative path is taken from the current directory inside the repository,
  a path starting with "/" (or any path with -R) from the repository root.

--settings, --wiki, --releases, --actions and --projects open those pages of
the repository and take no argument.

--commit without a SHA opens the last commit of the current branch (of the
default branch with -R), read with one API call; give a SHA as --commit=<sha>.
A path without --branch or --commit reads the default branch with one API call.
Everything else makes no API call and needs no token. The browser is the
command in COPIA_BROWSER, the config key browser or BROWSER, otherwise the
system's default opener.`,
		Example: `  # Open the current repository
  $ copia-cli browse

  # Open issue or pull request 42
  $ copia-cli browse 42

  # Open a commit
  $ copia-cli browse 77507cd94ccafcf568f8560cfecde965fcfa63

  # Open main.st at line 312
  $ copia-cli browse main.st:312

  # Open the blame view of main.st at line 312
  $ copia-cli browse main.st:312 --blame

  # Open main.st on the bug-fix branch
  $ copia-cli browse main.st --branch bug-fix

  # Open main.st at a commit
  $ copia-cli browse main.st --commit=77507cd94ccafcf568f8560cfecde965fcfa63

  # Open the repository settings
  $ copia-cli browse --settings

  # Print the URL of a pull request in another repository
  $ copia-cli browse 7 -R my-org/my-repo --no-browser`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Browser = f.Browser
			opts.HasRepoOverride = f.Repo != ""
			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if err := validate(opts); err != nil {
				return err
			}

			var err error
			if needsAPI(opts) {
				opts.Host, opts.Token, err = f.ResolveAuth()
				opts.HTTPClient = cmdutil.NewHTTPClient()
			} else {
				opts.Host, err = f.ResolveHost()
			}
			if err != nil {
				return err
			}

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return fmt.Errorf("could not determine repository: %w", err)
			}
			opts.Owner, opts.Repo = owner, repo

			return BrowseRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Projects, "projects", "p", false, "Open repository projects")
	cmd.Flags().BoolVarP(&opts.Releases, "releases", "r", false, "Open repository releases")
	cmd.Flags().BoolVarP(&opts.Wiki, "wiki", "w", false, "Open repository wiki")
	cmd.Flags().BoolVarP(&opts.Actions, "actions", "a", false, "Open repository actions")
	cmd.Flags().BoolVarP(&opts.Settings, "settings", "s", false, "Open repository settings")
	cmd.Flags().BoolVar(&opts.Blame, "blame", false, "Open blame view for a file")
	cmd.Flags().BoolVarP(&opts.NoBrowser, "no-browser", "n", false, "Print destination URL instead of opening the browser")
	cmd.Flags().StringVarP(&opts.Commit, "commit", "c", "", "Select another commit by passing in the commit SHA, default is the last commit")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Select another branch by passing in the branch name")
	cmd.Flags().Lookup("commit").NoOptDefVal = lastCommit

	return cmd
}

// validate rejects the flag and argument combinations gh rejects, before
// anything is resolved or opened.
func validate(opts *BrowseOptions) error {
	section := opts.Projects || opts.Releases || opts.Settings || opts.Wiki || opts.Actions
	if opts.SelectorArg != "" && section {
		return fmt.Errorf("arguments not supported when using --projects, --releases, --settings, --actions or --wiki")
	}
	set := 0
	for _, on := range []bool{opts.Branch != "", opts.Commit != "", opts.Projects, opts.Releases, opts.Settings, opts.Wiki, opts.Actions} {
		if on {
			set++
		}
	}
	if set > 1 {
		return fmt.Errorf("specify only one of --branch, --commit, --projects, --releases, --settings, --actions or --wiki")
	}
	if opts.Blame && opts.SelectorArg == "" {
		return fmt.Errorf("--blame requires a file path argument")
	}
	if (isNumber(opts.SelectorArg) || isCommit(opts.SelectorArg)) && (opts.Branch != "" || opts.Commit != "") {
		return fmt.Errorf("%q is an invalid argument when using --branch or --commit", opts.SelectorArg)
	}
	if isNumber(opts.SelectorArg) {
		if n, _ := strconv.Atoi(strings.TrimPrefix(opts.SelectorArg, "#")); n < 1 {
			return fmt.Errorf("invalid argument %q: expected an issue or pull request number", opts.SelectorArg)
		}
	}
	if isPath(opts) {
		if _, _, _, err := parseFile(opts.SelectorArg); err != nil {
			return err
		}
	}
	return nil
}

// isPath reports whether the argument names a file or directory.
func isPath(opts *BrowseOptions) bool {
	if opts.SelectorArg == "" {
		return false
	}
	if opts.Branch != "" || opts.Commit != "" {
		return true
	}
	return !isNumber(opts.SelectorArg) && !isCommit(opts.SelectorArg)
}

// needsAPI reports whether the URL needs the default branch or the last
// commit from the API.
func needsAPI(opts *BrowseOptions) bool {
	return opts.Commit == lastCommit || (isPath(opts) && opts.Branch == "" && opts.Commit == "")
}

// BrowseRun opens, or with NoBrowser prints, the URL that opts selects.
func BrowseRun(opts *BrowseOptions) error {
	section, err := parseSection(opts)
	if err != nil {
		return err
	}
	u := cmdutil.RepoWebURL(opts.Host, opts.Owner, opts.Repo, section)

	if opts.NoBrowser {
		_, err := fmt.Fprintln(opts.IO.Out, u)
		return err
	}

	if opts.IO.IsStdoutTTY() {
		_, _ = fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", strings.TrimPrefix(u, "https://"))
	}
	return opts.Browser.Browse(u)
}

// parseSection returns the part of the URL after the repository, in the
// layout of the Gitea web routes.
func parseSection(opts *BrowseOptions) (string, error) {
	switch {
	case opts.Projects:
		return "projects", nil
	case opts.Releases:
		return "releases", nil
	case opts.Settings:
		return "settings", nil
	case opts.Wiki:
		return "wiki", nil
	case opts.Actions:
		return "actions", nil
	}

	if !isPath(opts) && opts.Branch == "" && opts.Commit == "" {
		switch {
		case opts.SelectorArg == "":
			return "", nil
		case isNumber(opts.SelectorArg):
			return "issues/" + strings.TrimPrefix(opts.SelectorArg, "#"), nil
		default:
			return "commit/" + opts.SelectorArg, nil
		}
	}

	refType, ref := "branch", opts.Branch
	if opts.Commit != "" {
		refType, ref = "commit", opts.Commit
	}
	var err error
	switch {
	case opts.Commit == lastCommit:
		ref, err = lastCommitSHA(opts)
	case ref == "":
		ref, err = defaultBranch(opts)
	}
	if err != nil {
		return "", err
	}

	filePath, start, end, err := parseFile(opts.SelectorArg)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(filePath, "/") && !opts.HasRepoOverride && filePath != "" {
		filePath = path.Join(pathFromRepoRoot(), filePath)
		if filePath == "." || strings.HasPrefix(filePath, "..") {
			filePath = ""
		}
	}
	filePath = strings.Trim(path.Clean("/"+filePath), "/")

	view := "src"
	if opts.Blame {
		view = "blame"
	}
	section := strings.TrimSuffix(fmt.Sprintf("%s/%s/%s/%s", view, refType, escapePath(ref), escapePath(filePath)), "/")
	if start > 0 {
		fragment := fmt.Sprintf("L%d", start)
		if end > start {
			fragment += fmt.Sprintf("-L%d", end)
		}
		if !opts.Blame {
			// Show the source, not the rendered file, so the line exists.
			section += "?display=source"
		}
		section += "#" + fragment
	}
	return section, nil
}

// escapePath escapes p for a URL path and keeps its slashes.
func escapePath(p string) string {
	return strings.ReplaceAll(url.PathEscape(p), "%2F", "/")
}

// parseFile splits "<path>", "<path>:<line>" or "<path>:<start>-<end>".
func parseFile(f string) (p string, start, end int, err error) {
	parts := strings.SplitN(f, ":", 3)
	if len(parts) > 2 {
		return "", 0, 0, fmt.Errorf("invalid file argument: %q", f)
	}
	p = filepath.ToSlash(parts[0])
	if len(parts) < 2 {
		return p, 0, 0, nil
	}

	lines := parts[1]
	startText, endText, isRange := strings.Cut(lines, "-")
	start, err = strconv.Atoi(startText)
	if err == nil && isRange {
		end, err = strconv.Atoi(endText)
	} else {
		end = start
	}
	if err != nil || start < 1 || end < start {
		return "", 0, 0, fmt.Errorf("invalid file argument: %q", f)
	}
	return p, start, end, nil
}

func isNumber(arg string) bool {
	_, err := strconv.Atoi(strings.TrimPrefix(arg, "#"))
	return err == nil
}

// commitHash matches a SHA-1 or SHA-256 commit hash or a prefix of one.
var commitHash = regexp.MustCompile(`\A[a-f0-9]{7,64}\z`)

func isCommit(arg string) bool {
	return commitHash.MatchString(arg)
}

// pathFromRepoRoot returns the current directory relative to the root of
// the git repository, or "" outside of one.
func pathFromRepoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-prefix").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// currentBranch returns the checked-out branch of the git repository.
func currentBranch() (string, error) {
	out, err := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// lastCommitSHA returns the SHA of the tip of the current branch, or of the
// default branch with -R or outside a branch, with GET /branches/{branch}.
func lastCommitSHA(opts *BrowseOptions) (string, error) {
	branch := ""
	if !opts.HasRepoOverride {
		branch, _ = currentBranch()
	}
	if branch == "" {
		var err error
		if branch, err = defaultBranch(opts); err != nil {
			return "", err
		}
	}

	var result struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	notFound := fmt.Sprintf("branch %s not found in %s/%s; push it first or pass --commit=<sha>", branch, opts.Owner, opts.Repo)
	if err := getJSON(opts, "branches/"+escapePath(branch), notFound, &result); err != nil {
		return "", err
	}
	if result.Commit.ID == "" {
		return "", fmt.Errorf("could not determine the last commit of branch %s", branch)
	}
	return result.Commit.ID, nil
}

// defaultBranch returns the default branch of the repository.
func defaultBranch(opts *BrowseOptions) (string, error) {
	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	notFound := fmt.Sprintf("repository %s/%s not found", opts.Owner, opts.Repo)
	if err := getJSON(opts, "", notFound, &result); err != nil {
		return "", fmt.Errorf("error determining the default branch: %w", err)
	}
	if result.DefaultBranch == "" {
		return "", fmt.Errorf("error determining the default branch: repository %s/%s has none", opts.Owner, opts.Repo)
	}
	return result.DefaultBranch, nil
}

// getJSON reads GET /repos/{owner}/{repo}[/sub] into v.
func getJSON(opts *BrowseOptions, sub, notFound string, v any) error {
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", opts.Host, url.PathEscape(opts.Owner), url.PathEscape(opts.Repo))
	if sub != "" {
		u += "/" + sub
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("%s", notFound)
	default:
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	return nil
}
