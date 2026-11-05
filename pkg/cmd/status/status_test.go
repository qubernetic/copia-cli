package status

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	searchPath        = "/api/v1/repos/issues/search"
	notificationsPath = "/api/v1/notifications"
)

// hit is one result of the issue search.
type hit struct {
	repo    string
	number  int
	title   string
	updated string
	pull    bool
}

func hits(hs ...hit) string {
	items := make([]string, 0, len(hs))
	for _, h := range hs {
		pr := "null"
		if h.pull {
			pr = `{"merged":false}`
		}
		items = append(items, fmt.Sprintf(`{"number":%d,"title":%q,"updated_at":%q,"repository":{"full_name":%q},"pull_request":%s}`,
			h.number, h.title, h.updated, h.repo, pr))
	}
	return "[" + strings.Join(items, ",") + "]"
}

// note is one notification thread.
type note struct {
	repo  string
	kind  string // Issue, Pull, Commit or Repository
	title string
	path  string // web path after the repository, e.g. issues/3
}

func notes(ns ...note) string {
	items := make([]string, 0, len(ns))
	for _, n := range ns {
		htmlURL := "https://app.copia.io/" + n.repo
		if n.path != "" {
			htmlURL += "/" + n.path
		}
		items = append(items, fmt.Sprintf(`{"id":1,"subject":{"title":%q,"type":%q,"html_url":%q},"repository":{"full_name":%q}}`,
			n.title, n.kind, htmlURL, n.repo))
	}
	return "[" + strings.Join(items, ",") + "]"
}

// stubs are the four API answers of one status run.
type stubs struct {
	assigned, review, mentioned, activity string
	query                                 url.Values // extra parameters every search must carry
}

func register(reg *httpmock.Registry, s stubs) {
	for _, q := range []struct {
		params url.Values
		body   string
	}{
		{url.Values{"assigned": {"true"}, "state": {"open"}}, s.assigned},
		{url.Values{"review_requested": {"true"}, "type": {"pulls"}, "state": {"open"}}, s.review},
		{url.Values{"mentioned": {"true"}, "state": {"open"}}, s.mentioned},
	} {
		for k, v := range s.query {
			q.params[k] = v
		}
		reg.Register(httpmock.QueryMatcher("GET", searchPath, q.params), httpmock.StringResponse(http.StatusOK, q.body))
	}
	reg.Register(httpmock.QueryMatcher("GET", notificationsPath, url.Values{"all": {"true"}}),
		httpmock.StringResponse(http.StatusOK, s.activity))
}

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams) *StatusOptions {
	return &StatusOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
	}
}

func TestStatusRun(t *testing.T) {
	tests := []struct {
		name  string
		tty   bool
		width int
		stubs stubs
		want  string
	}{
		{
			name: "every section",
			stubs: stubs{
				assigned: hits(
					hit{repo: "my-org/plc", number: 3, title: "Fix PLC timeout", updated: "2026-11-04T10:00:00Z"},
					hit{repo: "my-org/plc", number: 7, title: "Add interlock", updated: "2026-11-03T10:00:00Z", pull: true},
					hit{repo: "my-org/plc", number: 5, title: "Newer", updated: "2026-11-05T10:00:00Z"},
				),
				review:    hits(hit{repo: "other/hmi", number: 2, title: "Review the HMI screens for the packaging line", updated: "2026-11-01T10:00:00Z", pull: true}),
				mentioned: hits(hit{repo: "my-org/scada", number: 11, title: "Alarm\r\nlimits", updated: "2026-11-02T10:00:00Z"}),
				activity: notes(
					note{repo: "my-org/plc", kind: "Issue", title: "Fix PLC timeout", path: "issues/3"},
					note{repo: "other/hmi", kind: "Pull", title: "Review the HMI screens for the packaging line", path: "pulls/2"},
					note{repo: "my-org/plc", kind: "Commit", title: "Fix typo", path: "commit/abcdef1234567890"},
					note{repo: "my-org/scada", kind: "Repository", title: "Transferred to my-org"},
				),
			},
			want: `Assigned Issues                       │ Assigned Pull Requests
my-org/plc#5  Newer                   │ my-org/plc#7  Add interlock
my-org/plc#3  Fix PLC timeout         │
                                      │
Review Requests                       │ Mentions
other/hmi#2  Review the HMI screens...│ my-org/scada#11  Alarm limits
                                      │
Repository Activity
my-org/plc#3        issue       Fix PLC timeout
other/hmi#2         PR          Review the HMI screens for the packaging line
my-org/plc@abcdef1  commit      Fix typo
my-org/scada        repository  Transferred to my-org

`,
		},
		{
			name:  "empty sections",
			stubs: stubs{assigned: "[]", review: "[]", mentioned: "[]", activity: "[]"},
			want: `Assigned Issues                       │ Assigned Pull Requests
Nothing here ^_^                      │ Nothing here ^_^
                                      │
Review Requests                       │ Mentions
Nothing here ^_^                      │ Nothing here ^_^
                                      │
Repository Activity
Nothing here ^_^

`,
		},
		{
			name:  "terminal width",
			tty:   true,
			width: 60,
			stubs: stubs{
				assigned:  hits(hit{repo: "my-org/plc", number: 3, title: "Fix PLC timeout on line three", updated: "2026-11-04T10:00:00Z"}),
				review:    "[]",
				mentioned: "[]",
				activity:  notes(note{repo: "my-org/plc", kind: "Issue", title: "Fix PLC timeout on line three, which stops the conveyor", path: "issues/3"}),
			},
			want: `Assigned Issues             │ Assigned Pull Requests
my-org/plc#3  Fix PLC tim...│ Nothing here ^_^
                            │
Review Requests             │ Mentions
Nothing here ^_^            │ Nothing here ^_^
                            │
Repository Activity
my-org/plc#3  issue  Fix PLC timeout on line three, which...

`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			register(reg, tt.stubs)

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			if tt.width > 0 {
				ios.SetTerminalWidth(tt.width)
			}
			require.NoError(t, StatusRun(newOpts(reg, ios)))
			assert.Equal(t, tt.want, stdout.String())
		})
	}
}

func TestStatusRun_RowLimits(t *testing.T) {
	var issues []hit
	for n := 1; n <= 7; n++ {
		issues = append(issues, hit{repo: "my-org/plc", number: n, title: fmt.Sprintf("Issue %d", n), updated: fmt.Sprintf("2026-11-%02dT10:00:00Z", n)})
	}
	var activity []note
	for n := 1; n <= 12; n++ {
		activity = append(activity, note{repo: "my-org/plc", kind: "Issue", title: fmt.Sprintf("Activity %d", n), path: fmt.Sprintf("issues/%d", n)})
	}

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	register(reg, stubs{assigned: hits(issues...), review: "[]", mentioned: "[]", activity: notes(activity...)})

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, StatusRun(newOpts(reg, ios)))
	out := stdout.String()

	// The five most recently updated issues, newest first.
	for n := 7; n >= 3; n-- {
		assert.Contains(t, out, fmt.Sprintf("my-org/plc#%d  Issue %d", n, n))
	}
	assert.NotContains(t, out, "Issue 2")
	assert.NotContains(t, out, "Issue 1 ")
	assert.Less(t, strings.Index(out, "Issue 7"), strings.Index(out, "Issue 3"))

	// Ten rows of activity in the order of the server.
	assert.Contains(t, out, "Activity 10")
	assert.NotContains(t, out, "Activity 11")
	assert.Less(t, strings.Index(out, "Activity 1\n"), strings.Index(out, "Activity 10"))
}

func TestStatusRun_Org(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	register(reg, stubs{
		query:     url.Values{"owner": {"my-org"}},
		assigned:  hits(hit{repo: "my-org/plc", number: 3, title: "Fix PLC timeout", updated: "2026-11-04T10:00:00Z"}),
		review:    "[]",
		mentioned: "[]",
		activity: notes(
			note{repo: "My-Org/plc", kind: "Issue", title: "In the org", path: "issues/3"},
			note{repo: "other/hmi", kind: "Pull", title: "Elsewhere", path: "pulls/2"},
		),
	})

	ios, _, stdout, _ := iostreams.Test()
	opts := newOpts(reg, ios)
	opts.Org = "my-org"
	require.NoError(t, StatusRun(opts))
	assert.Contains(t, stdout.String(), "My-Org/plc#3  issue  In the org")
	assert.NotContains(t, stdout.String(), "Elsewhere")
}

func TestStatusRun_Exclude(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	register(reg, stubs{
		assigned: hits(
			hit{repo: "my-org/plc", number: 3, title: "Kept issue", updated: "2026-11-04T10:00:00Z"},
			hit{repo: "My-Org/Legacy", number: 4, title: "Dropped issue", updated: "2026-11-04T10:00:00Z"},
			hit{repo: "other/hmi", number: 8, title: "Dropped pull", updated: "2026-11-04T10:00:00Z", pull: true},
		),
		review:    hits(hit{repo: "other/hmi", number: 9, title: "Dropped review", updated: "2026-11-04T10:00:00Z", pull: true}),
		mentioned: hits(hit{repo: "my-org/legacy", number: 5, title: "Dropped mention", updated: "2026-11-04T10:00:00Z"}),
		activity: notes(
			note{repo: "my-org/legacy", kind: "Issue", title: "Dropped activity", path: "issues/5"},
			note{repo: "my-org/plc", kind: "Issue", title: "Kept activity", path: "issues/3"},
		),
	})

	ios, _, stdout, _ := iostreams.Test()
	opts := newOpts(reg, ios)
	opts.Exclude = []string{"my-org/legacy", "other/hmi"}
	require.NoError(t, StatusRun(opts))
	out := stdout.String()
	assert.Contains(t, out, "Kept issue")
	assert.Contains(t, out, "Kept activity")
	assert.NotContains(t, out, "Dropped")
	assert.Equal(t, 3, strings.Count(out, "Nothing here ^_^"), "assigned pull requests, review requests and mentions are empty")
}

func TestStatusRun_Errors(t *testing.T) {
	ok := httpmock.StringResponse(http.StatusOK, "[]")
	tests := []struct {
		name     string
		org      string
		exclude  []string
		search   httpmock.Responder
		activity httpmock.Responder
		wantErr  string
	}{
		{name: "invalid exclude", exclude: []string{"plc"}, wantErr: `invalid value for --exclude: "plc", expected owner/repo`},
		{name: "search fails", search: httpmock.StringResponse(http.StatusInternalServerError, ""), activity: ok, wantErr: "could not search for assignments: API error (HTTP 500)"},
		{name: "unknown org", org: "nobody", search: httpmock.StringResponse(http.StatusBadRequest, ""), activity: ok, wantErr: `could not search for assignments: no user or organization named "nobody"`},
		{name: "notifications fail", search: ok, activity: httpmock.StringResponse(http.StatusInternalServerError, ""), wantErr: "could not load notifications: API error (HTTP 500)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.search != nil {
				reg.Register(httpmock.REST("GET", searchPath), tt.search)
				reg.Register(httpmock.REST("GET", notificationsPath), tt.activity)
			}

			ios, _, stdout, _ := iostreams.Test()
			opts := newOpts(reg, ios)
			opts.Org = tt.org
			opts.Exclude = tt.exclude
			err := StatusRun(opts)
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
			assert.Empty(t, stdout.String())
			if tt.search == nil {
				assert.Empty(t, reg.Requests, "an invalid flag sends no request")
			}
		})
	}
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdStatus(f)
	cmd.SetArgs(args)
	cmd.SetOut(f.IOStreams.ErrOut)
	cmd.SetErr(f.IOStreams.ErrOut)
	return cmd.Execute()
}

func TestNewCmdStatus(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	register(reg, stubs{
		query:     url.Values{"owner": {"my-org"}},
		assigned:  hits(hit{repo: "my-org/a", number: 1, title: "Dropped a", updated: "2026-11-04T10:00:00Z"}, hit{repo: "my-org/c", number: 3, title: "Kept c", updated: "2026-11-04T10:00:00Z"}),
		review:    hits(hit{repo: "my-org/b", number: 2, title: "Dropped b", updated: "2026-11-04T10:00:00Z", pull: true}),
		mentioned: "[]",
		activity:  "[]",
	})

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	// --exclude takes a comma-separated list and repeats, as in gh.
	require.NoError(t, runCmd(f, "-o", "my-org", "-e", "my-org/a,my-org/x", "--exclude", "my-org/b"))
	assert.Contains(t, stdout.String(), "Kept c")
	assert.NotContains(t, stdout.String(), "Dropped")
}

func TestNewCmdStatus_Errors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	err := runCmd(f, "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestStatusRun_LaterSearchFails(t *testing.T) {
	for _, filter := range []string{"review_requested", "mentioned"} {
		t.Run(filter, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{filter: {"true"}}),
				httpmock.StringResponse(http.StatusInternalServerError, ""))
			reg.Register(httpmock.REST("GET", searchPath), httpmock.StringResponse(http.StatusOK, "[]"))

			ios, _, _, _ := iostreams.Test()
			err := StatusRun(newOpts(reg, ios))
			require.Error(t, err)
			assert.Equal(t, "could not search for assignments: API error (HTTP 500)", err.Error())
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"Fix PLC timeout", 20, "Fix PLC timeout"},
		{"Fix PLC timeout", 15, "Fix PLC timeout"},
		{"Fix PLC timeout", 10, "Fix PLC..."},
		{"Fix PLC timeout", 3, "Fix"},
		{"Fix PLC timeout", 0, ""},
		{"Fix PLC timeout", -4, ""},
		{"Hőmérő érzékelő", 8, "Hőmér..."},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, truncate(tt.in, tt.width), "%q at %d", tt.in, tt.width)
	}
}

func TestNewCmdStatus_NoToken(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) {
		return nil, errors.New("no config")
	}}
	err := runCmd(f)
	require.Error(t, err)
	assert.Equal(t, "no config", err.Error())
}
