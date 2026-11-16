package status

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const issuesPath = "/api/v1/repos/my-org/my-repo/issues"

var testNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func issueJSON(number int, title, updated, author string, assignees ...string) string {
	var as []string
	for _, a := range assignees {
		as = append(as, fmt.Sprintf(`{"login":%q}`, a))
	}
	return fmt.Sprintf(`{"number":%d,"title":%q,"state":"open","updated_at":%q,"html_url":"https://app.copia.io/my-org/my-repo/issues/%d","labels":[{"name":"bug"}],"user":{"login":%q},"assignees":[%s]}`,
		number, title, updated, number, author, strings.Join(as, ","))
}

func list(issues ...string) string {
	return "[" + strings.Join(issues, ",") + "]"
}

func registerSections(reg *httpmock.Registry, assigned, mentioned, authored string) {
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"jane"}`))
	for _, s := range []struct{ key, body string }{
		{"assigned_by", assigned},
		{"mentioned_by", mentioned},
		{"created_by", authored},
	} {
		reg.Register(httpmock.QueryMatcher("GET", issuesPath, url.Values{
			s.key: {"jane"}, "state": {"open"}, "type": {"issues"}, "page": {"1"},
		}), httpmock.StringResponse(http.StatusOK, s.body))
	}
}

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams) *StatusOptions {
	return &StatusOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Now:        func() time.Time { return testNow },
	}
}

func TestStatusRun(t *testing.T) {
	var many []string
	for n := 30; n < 42; n++ {
		many = append(many, issueJSON(n, fmt.Sprintf("Mention %d", n), "2026-11-01T12:00:00Z", "bob"))
	}

	tests := []struct {
		name      string
		tty       bool
		assigned  string
		mentioned string
		authored  string
		want      string
	}{
		{
			name: "three sections",
			tty:  true,
			assigned: list(
				issueJSON(12, "Fix PLC timeout", "2026-11-05T09:00:00Z", "bob", "jane"),
				issueJSON(14, "Newer assigned", "2026-11-05T11:00:00Z", "bob", "jane"),
			),
			mentioned: list(issueJSON(20, "Ladder logic review", "2026-11-03T12:00:00Z", "bob")),
			authored: list(
				issueJSON(12, "Fix PLC timeout", "2026-11-05T09:00:00Z", "bob", "jane"),
				issueJSON(21, "Mine", "2026-10-05T12:00:00Z", "jane"),
			),
			want: `
Relevant issues in my-org/my-repo

Issues assigned to you
  #14  Newer assigned   bug  1 hour ago
  #12  Fix PLC timeout  bug  3 hours ago

Issues mentioning you
  #20  Ladder logic review  bug  2 days ago

Issues opened by you
  #21  Mine  bug  1 month ago

`,
		},
		{
			name:      "empty sections",
			tty:       true,
			assigned:  "[]",
			mentioned: "[]",
			// Copia ignores created_by, so issues of others come back too.
			authored: list(issueJSON(12, "Not mine", "2026-11-05T09:00:00Z", "bob")),
			want: `
Relevant issues in my-org/my-repo

Issues assigned to you
  There are no issues assigned to you

Issues mentioning you
  There are no issues mentioning you

Issues opened by you
  There are no issues opened by you

`,
		},
		{
			name: "unassigned issue dropped",
			tty:  true,
			// An assigned_by the server ignores must not list it.
			assigned:  list(issueJSON(12, "Not mine", "2026-11-05T09:00:00Z", "bob", "bob")),
			mentioned: "[]",
			authored:  "[]",
			want: `
Relevant issues in my-org/my-repo

Issues assigned to you
  There are no issues assigned to you

Issues mentioning you
  There are no issues mentioning you

Issues opened by you
  There are no issues opened by you

`,
		},
		{
			name:      "more than ten",
			tty:       true,
			assigned:  "[]",
			mentioned: list(many...),
			authored:  "[]",
			want: `
Relevant issues in my-org/my-repo

Issues assigned to you
  There are no issues assigned to you

Issues mentioning you
  #30  Mention 30  bug  4 days ago
  #31  Mention 31  bug  4 days ago
  #32  Mention 32  bug  4 days ago
  #33  Mention 33  bug  4 days ago
  #34  Mention 34  bug  4 days ago
  #35  Mention 35  bug  4 days ago
  #36  Mention 36  bug  4 days ago
  #37  Mention 37  bug  4 days ago
  #38  Mention 38  bug  4 days ago
  #39  Mention 39  bug  4 days ago
  And 2 more

Issues opened by you
  There are no issues opened by you

`,
		},
		{
			name:      "not a terminal",
			tty:       false,
			assigned:  list(issueJSON(12, "Fix PLC timeout", "2026-11-05T09:00:00Z", "bob", "jane")),
			mentioned: "[]",
			authored:  "[]",
			want: `
Relevant issues in my-org/my-repo

Issues assigned to you
  12  open  Fix PLC timeout  bug  2026-11-05T09:00:00Z

Issues mentioning you
  There are no issues mentioning you

Issues opened by you
  There are no issues opened by you

`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			registerSections(reg, tt.assigned, tt.mentioned, tt.authored)

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			require.NoError(t, StatusRun(newOpts(reg, ios)))
			assert.Equal(t, tt.want, stdout.String())
		})
	}
}

func TestStatusRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	registerSections(reg,
		list(issueJSON(12, "Fix PLC timeout", "2026-11-05T09:00:00Z", "bob", "jane")),
		"[]",
		list(issueJSON(12, "Fix PLC timeout", "2026-11-05T09:00:00Z", "bob", "jane"), issueJSON(21, "Mine", "2026-10-05T12:00:00Z", "jane")),
	)

	ios, _, stdout, _ := iostreams.Test()
	opts := newOpts(reg, ios)
	opts.JSON = cmdutil.JSONFlags{Fields: []string{"number", "title"}}

	require.NoError(t, StatusRun(opts))
	assert.JSONEq(t, `{
		"assigned": [{"number": 12, "title": "Fix PLC timeout"}],
		"mentioned": [],
		"createdBy": [{"number": 21, "title": "Mine"}]
	}`, stdout.String())
}

func TestStatusRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		user    httpmock.Responder
		issues  httpmock.Responder
		wantErr string
	}{
		{
			name:    "current user",
			user:    httpmock.StringResponse(http.StatusUnauthorized, `{}`),
			wantErr: "HTTP 401",
		},
		{
			name:    "issues",
			user:    httpmock.StringResponse(http.StatusOK, `{"login":"jane"}`),
			issues:  httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "API error (HTTP 404)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(httpmock.REST("GET", "/api/v1/user"), tt.user)
			if tt.issues != nil {
				reg.Register(httpmock.REST("GET", issuesPath), tt.issues)
			}

			ios, _, stdout, _ := iostreams.Test()
			err := StatusRun(newOpts(reg, ios))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdStatus(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdStatus(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerSections(reg, "[]", "[]", list(issueJSON(21, "Mine", "2026-10-05T12:00:00Z", "jane")))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runCmd(f, "--json", "number", "--jq", ".createdBy[].number"))
	assert.Equal(t, "21\n", stdout.String())
}

func TestNewCmdStatus_Errors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	err := runCmd(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}, "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")

	err = runCmd(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "bad"})
	require.Error(t, err)
}
