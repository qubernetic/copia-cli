package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// sectionSize is how many issues a section shows, as in gh.
const sectionSize = 10

var validJSONFields = []string{"number", "title", "state", "labels", "updated_at", "html_url", "user", "assignees"}

// StatusOptions holds all inputs for the issue status command.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	JSON       cmdutil.JSONFlags
	// Now is the clock the UPDATED column measures against.
	Now func() time.Time
}

type userRef struct {
	Login string `json:"login"`
}

type labelRef struct {
	Name string `json:"name"`
}

type issueEntry struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Labels    []labelRef `json:"labels"`
	UpdatedAt string     `json:"updated_at"`
	HTMLURL   string     `json:"html_url"`
	User      userRef    `json:"user"`
	Assignees []userRef  `json:"assignees"`
}

// section is one list of the status output.
type section struct {
	key    string // JSON key, as in gh
	header string
	empty  string
	issues []issueEntry
}

// NewCmdStatus creates the `copia issue status` command.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of relevant issues",
		Long: `Show the open issues of the repository that are relevant to you: the ones
assigned to you, the ones mentioning you and the ones you opened. Each
section shows the ten most recently updated issues.

With --json, the output is one object with the keys assigned, mentioned and
createdBy, each a list of issues with the requested fields.`,
		Example: `  $ copia-cli issue status
  $ copia-cli issue status -R my-org/my-repo
  $ copia-cli issue status --json number,title --jq '.assigned[].number'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			return StatusRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodeIssues(body []byte) ([]issueEntry, error) {
	var issues []issueEntry
	err := json.Unmarshal(body, &issues)
	return issues, err
}

// StatusRun fetches the three sections and prints them. Each section is one
// listing of the open issues with the assigned_by, mentioned_by or
// created_by filter. Copia ignores created_by, so the author is checked
// on every issue; the assignees are checked the same way, so an ignored
// assigned_by cannot list issues of others either.
func StatusRun(opts *StatusOptions) error {
	users, err := shared.ResolveAssignees(opts.HTTPClient, opts.Host, opts.Token, []string{"@me"})
	if err != nil {
		return err
	}
	me := users[0]

	isAssigned := func(i issueEntry) bool {
		return slices.ContainsFunc(i.Assignees, func(u userRef) bool { return u.Login == me })
	}
	isAuthor := func(i issueEntry) bool { return i.User.Login == me }

	sections := []*section{
		{key: "assigned", header: "Issues assigned to you", empty: "There are no issues assigned to you"},
		{key: "mentioned", header: "Issues mentioning you", empty: "There are no issues mentioning you"},
		{key: "createdBy", header: "Issues opened by you", empty: "There are no issues opened by you"},
	}
	for i, filter := range []struct {
		param string
		keep  func(issueEntry) bool
	}{
		{"assigned_by", isAssigned},
		{"mentioned_by", nil},
		{"created_by", isAuthor},
	} {
		u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues?", opts.Host, opts.Owner, opts.Repo) +
			url.Values{"state": {"open"}, "type": {"issues"}, filter.param: {me}}.Encode()
		var issues []issueEntry
		if filter.keep == nil {
			issues, err = cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, 0, decodeIssues)
		} else {
			issues, err = cmdutil.FetchFilteredPages(opts.HTTPClient, u, opts.Token, 0, decodeIssues, filter.keep)
		}
		if err != nil {
			return err
		}
		sortByUpdated(issues)
		sections[i].issues = issues
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.JSON.IsJSON() {
		data := make(map[string]any, len(sections))
		for _, s := range sections {
			data[s.key] = s.issues[:min(len(s.issues), sectionSize)]
		}
		return opts.JSON.PrintSections(opts.IO.Out, data)
	}

	out := opts.IO.Out
	_, _ = fmt.Fprintf(out, "\nRelevant issues in %s/%s\n\n", opts.Owner, opts.Repo)
	for _, s := range sections {
		_, _ = fmt.Fprintln(out, s.header)
		if len(s.issues) == 0 {
			_, _ = fmt.Fprintf(out, "  %s\n", s.empty)
		} else if err := printIssues(opts, s.issues); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out)
	}
	return nil
}

// sortByUpdated orders issues by their last update, newest first, as gh
// does; the Copia issue list has no sort parameter.
func sortByUpdated(issues []issueEntry) {
	sort.SliceStable(issues, func(a, b int) bool {
		ta, _ := time.Parse(time.RFC3339, issues[a].UpdatedAt)
		tb, _ := time.Parse(time.RFC3339, issues[b].UpdatedAt)
		return ta.After(tb)
	})
}

// printIssues prints the first sectionSize issues indented, as gh's issue
// table does: on a terminal the number with # and the update time in words,
// otherwise the state as an extra column and the raw update time.
func printIssues(opts *StatusOptions, issues []issueEntry) error {
	tty := opts.IO.IsStdoutTTY()
	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, i := range issues[:min(len(issues), sectionSize)] {
		names := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			names = append(names, l.Name)
		}
		labels := strings.Join(names, ", ")
		if tty {
			updated := i.UpdatedAt
			if t, err := time.Parse(time.RFC3339, i.UpdatedAt); err == nil {
				updated = cmdutil.TimeAgo(opts.Now(), t)
			}
			_, _ = fmt.Fprintf(w, "  #%d\t%s\t%s\t%s\n", i.Number, i.Title, labels, updated)
		} else {
			_, _ = fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s\n", i.Number, i.State, i.Title, labels, i.UpdatedAt)
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if more := len(issues) - sectionSize; more > 0 {
		_, _ = fmt.Fprintf(opts.IO.Out, "  And %d more\n", more)
	}
	return nil
}
