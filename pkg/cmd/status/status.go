// Package status implements `copia-cli status`, gh's dashboard of the
// issues, pull requests and notifications relevant to you across
// repositories.
package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

const (
	// searchLimit is how many results gh reads from each search.
	searchLimit = 25
	// halfRows and fullRows are the rows gh shows in a half-width section
	// and in Repository Activity.
	halfRows = 5
	fullRows = 10
)

// StatusOptions holds all inputs for the status command.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Org        string
	Exclude    []string
}

// searchHit is one result of the issue search.
type searchHit struct {
	Number     int64  `json:"number"`
	Title      string `json:"title"`
	UpdatedAt  string `json:"updated_at"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest *struct{} `json:"pull_request"`
}

// notification is one notification thread.
type notification struct {
	Subject struct {
		Title   string `json:"title"`
		Type    string `json:"type"`
		HTMLURL string `json:"html_url"`
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// item is one row of a section.
type item struct {
	identifier string // owner/repo#123
	reason     string // only used in Repository Activity
	preview    string
}

// NewCmdStatus creates the `copia-cli status` command.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print information about relevant issues, pull requests, and notifications across repositories",
		Long: `The status command prints information about your work on Copia across all
the repositories you can see, including:

- Assigned Issues
- Assigned Pull Requests
- Review Requests
- Mentions
- Repository Activity (your notifications)

The first four list open issues and pull requests, the five most recently
updated of each; Repository Activity lists your ten most recent notifications,
read or unread.`,
		Example: `  $ copia-cli status -e my-org/plc -e my-org/hmi # Exclude multiple repositories
  $ copia-cli status -o my-org # Limit results to a single organization`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return StatusRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "Report status within an organization")
	cmd.Flags().StringSliceVarP(&opts.Exclude, "exclude", "e", []string{}, "Comma separated list of repos to exclude in owner/name format")

	return cmd
}

// StatusRun loads the five sections and prints them as gh does: two pairs of
// half-width sections side by side, then Repository Activity.
func StatusRun(opts *StatusOptions) error {
	for _, repo := range opts.Exclude {
		if _, _, err := cmdutil.SplitOwnerRepo(repo); err != nil {
			return fmt.Errorf("invalid value for --exclude: %q, expected owner/repo", repo)
		}
	}

	assignments, err := opts.search(url.Values{"assigned": {"true"}})
	if err != nil {
		return err
	}
	var assignedIssues, assignedPRs []searchHit
	for _, h := range assignments {
		if h.PullRequest != nil {
			assignedPRs = append(assignedPRs, h)
		} else {
			assignedIssues = append(assignedIssues, h)
		}
	}
	reviewRequests, err := opts.search(url.Values{"review_requested": {"true"}, "type": {"pulls"}})
	if err != nil {
		return err
	}
	mentions, err := opts.search(url.Values{"mentioned": {"true"}})
	if err != nil {
		return err
	}
	activity, err := opts.notifications()
	if err != nil {
		return fmt.Errorf("could not load notifications: %w", err)
	}

	width := opts.IO.TerminalWidth()
	halfWidth := width/2 - 2

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	out := opts.IO.Out
	printColumns(out, halfWidth,
		section("Assigned Issues", hitItems(assignedIssues), halfWidth, halfRows),
		section("Assigned Pull Requests", hitItems(assignedPRs), halfWidth, halfRows))
	printColumns(out, halfWidth,
		section("Review Requests", hitItems(reviewRequests), halfWidth, halfRows),
		section("Mentions", hitItems(mentions), halfWidth, halfRows))
	for _, line := range section("Repository Activity", activity, width, fullRows) {
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}

// keep reports whether a result of the repository fullName belongs in the
// output: owned by --org, when given, and not excluded. Copia matches
// owners and repositories regardless of case, so this does too.
func (opts *StatusOptions) keep(fullName string) bool {
	if opts.Org != "" {
		owner, _, _ := strings.Cut(fullName, "/")
		if !strings.EqualFold(owner, opts.Org) {
			return false
		}
	}
	for _, repo := range opts.Exclude {
		if strings.EqualFold(repo, fullName) {
			return false
		}
	}
	return true
}

// search returns up to searchLimit open issues and pull requests of the
// issue search with filter, newest update first. --org is passed to the
// server as owner and checked again on each result, --exclude on the client.
func (opts *StatusOptions) search(filter url.Values) ([]searchHit, error) {
	q := url.Values{"state": {"open"}}
	for k, v := range filter {
		q[k] = v
	}
	if opts.Org != "" {
		q.Set("owner", opts.Org)
	}
	u := fmt.Sprintf("https://%s/api/v1/repos/issues/search?%s", opts.Host, q.Encode())
	hits, err := cmdutil.FetchFilteredPages(opts.HTTPClient, u, opts.Token, searchLimit, decodeHits, func(h searchHit) bool {
		return opts.keep(h.Repository.FullName)
	})
	if err != nil {
		var httpErr *cmdutil.HTTPError
		if opts.Org != "" && errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest {
			err = fmt.Errorf("no user or organization named %q", opts.Org)
		}
		return nil, fmt.Errorf("could not search for assignments: %w", err)
	}
	sort.SliceStable(hits, func(a, b int) bool {
		ta, _ := time.Parse(time.RFC3339, hits[a].UpdatedAt)
		tb, _ := time.Parse(time.RFC3339, hits[b].UpdatedAt)
		return ta.After(tb)
	})
	return hits, nil
}

// notifications returns the most recent notifications, read or unread, in
// the order of the server (newest first) as Repository Activity rows.
func (opts *StatusOptions) notifications() ([]item, error) {
	u := fmt.Sprintf("https://%s/api/v1/notifications?all=true", opts.Host)
	threads, err := cmdutil.FetchFilteredPages(opts.HTTPClient, u, opts.Token, fullRows, decodeNotifications, func(n notification) bool {
		return opts.keep(n.Repository.FullName)
	})
	if err != nil {
		return nil, err
	}
	items := make([]item, 0, len(threads))
	for _, n := range threads {
		repo := n.Repository.FullName
		last := path.Base(n.Subject.HTMLURL)
		it := item{identifier: repo, preview: n.Subject.Title}
		switch n.Subject.Type {
		case "Issue":
			it.identifier, it.reason = repo+"#"+last, "issue"
		case "Pull":
			it.identifier, it.reason = repo+"#"+last, "PR"
		case "Commit":
			it.identifier, it.reason = repo+"@"+last[:min(len(last), 7)], "commit"
		default:
			it.reason = strings.ToLower(n.Subject.Type)
		}
		items = append(items, it)
	}
	return items, nil
}

func decodeHits(body []byte) ([]searchHit, error) {
	var hits []searchHit
	err := json.Unmarshal(body, &hits)
	return hits, err
}

func decodeNotifications(body []byte) ([]notification, error) {
	var threads []notification
	err := json.Unmarshal(body, &threads)
	return threads, err
}

func hitItems(hits []searchHit) []item {
	items := make([]item, 0, len(hits))
	for _, h := range hits {
		items = append(items, item{
			identifier: fmt.Sprintf("%s#%d", h.Repository.FullName, h.Number),
			preview:    h.Title,
		})
	}
	return items
}

// section renders a header and up to rows items as gh's table does: the
// columns two spaces apart, the identifier never cut and the preview cut to
// width. An empty section reads "Nothing here ^_^". The last line is empty.
func section(header string, items []item, width, rows int) []string {
	lines := []string{header}
	if len(items) == 0 {
		return append(lines, "Nothing here ^_^", "")
	}
	items = items[:min(len(items), rows)]
	idWidth, reasonWidth := 0, 0
	for _, it := range items {
		idWidth = max(idWidth, utf8.RuneCountInString(it.identifier))
		reasonWidth = max(reasonWidth, utf8.RuneCountInString(it.reason))
	}
	for _, it := range items {
		line := pad(it.identifier, idWidth) + "  "
		if reasonWidth > 0 {
			line += pad(it.reason, reasonWidth) + "  "
		}
		preview := strings.ReplaceAll(strings.ReplaceAll(it.preview, "\r", ""), "\n", " ")
		line += truncate(preview, width-utf8.RuneCountInString(line))
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return append(lines, "")
}

// printColumns prints left and right side by side as gh's lipgloss layout
// does: left padded to width, a vertical line, a space and right. Trailing
// blanks are dropped.
func printColumns(out io.Writer, width int, left, right []string) {
	for i := range max(len(left), len(right)) {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		_, _ = fmt.Fprintln(out, strings.TrimRight(pad(l, width)+"│ "+r, " "))
	}
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

// truncate cuts s to width runes, ending in "..." when it was cut, as gh
// does.
func truncate(s string, width int) string {
	r := []rune(s)
	switch {
	case len(r) <= width:
		return s
	case width <= 3:
		return string(r[:max(0, width)])
	default:
		return string(r[:width-3]) + "..."
	}
}
