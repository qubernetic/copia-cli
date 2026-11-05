package list

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const issuesPath = "/api/v1/repos/my-org/my-repo/issues"

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdList(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdList_Defaults(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuesPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios))
	require.NoError(t, err)
	assert.Empty(t, stdout.String())

	require.Len(t, reg.Requests, 1)
	q := reg.Requests[0].URL.Query()
	assert.Equal(t, "open", q.Get("state"))
	assert.Equal(t, "30", q.Get("limit"))
	assert.False(t, q.Has("labels"))
}

func TestNewCmdList_Flags(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuesPath), httpmock.StringResponse(http.StatusOK,
		`[{"number":3,"title":"T","state":"closed","updated_at":"2026-03-30T10:00:00Z","labels":[{"name":"bug"}]}]`))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "-s", "closed", "-L", "5", "-l", "bug", "--json", "number,state")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"number": 3`)

	q := reg.Requests[0].URL.Query()
	assert.Equal(t, "closed", q.Get("state"))
	assert.Equal(t, "5", q.Get("limit"))
	assert.Equal(t, "bug", q.Get("labels"))
	assert.False(t, q.Has("assigned_by"))
	assert.False(t, q.Has("q"))
}

func TestNewCmdList_FilterFlags(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"me"}`))
	reg.Register(httpmock.REST("GET", issuesPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "-a", "@me", "-A", "jane", "--mention", "joe", "-m", "v1.0", "-S", "plc timeout")
	require.NoError(t, err)

	require.Len(t, reg.Requests, 2)
	q := reg.Requests[1].URL.Query()
	assert.Equal(t, "me", q.Get("assigned_by"))
	assert.Equal(t, "jane", q.Get("created_by"))
	assert.Equal(t, "joe", q.Get("mentioned_by"))
	assert.Equal(t, "v1.0", q.Get("milestones"))
	assert.Equal(t, "plc timeout", q.Get("q"))
}

func TestNewCmdList_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runCmd(t, f)
	require.EqualError(t, err, "no config")
}

func TestNewCmdList_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

func TestNewCmdList_InvalidLimitFlag(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "--limit", "0")
	require.EqualError(t, err, "invalid limit: 0")
}

func listOpts(reg *httpmock.Registry) (*ListOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "all",
		Limit:      30,
	}, stdout
}

func TestListRun_InvalidLimit(t *testing.T) {
	opts, _ := listOpts(&httpmock.Registry{})
	opts.Limit = -1
	require.EqualError(t, ListRun(opts), "invalid limit: -1")
}

func TestListRun_FetchErrors(t *testing.T) {
	tests := []struct {
		name      string
		responder httpmock.Responder
		wantErr   string
	}{
		{"server error", httpmock.StringResponse(http.StatusInternalServerError, `{}`), "API error (HTTP 500)"},
		{"malformed JSON", httpmock.StringResponse(http.StatusOK, `{not json`), "invalid character"},
		{"transport error", func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuesPath), tt.responder)

			opts, stdout := listOpts(reg)
			err := ListRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestListRun_TableJoinsLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", issuesPath), httpmock.StringResponse(http.StatusOK,
		`[{"number":7,"title":"Timer drift","state":"closed","labels":[{"name":"bug"},{"name":"plc"},{"name":"p1"}]}]`))

	opts, stdout := listOpts(reg)
	require.NoError(t, ListRun(opts))
	assert.Equal(t, "#7  Timer drift  closed  bug, plc, p1\n", stdout.String())
	assert.Equal(t, "all", reg.Requests[0].URL.Query().Get("state"))
}

func TestNewCmdList_JSONFieldsAndJQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuesPath), httpmock.StringResponse(http.StatusOK,
		`[{"number":3,"title":"T","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[]},
		  {"number":4,"title":"U","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[]}]`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "--json", "number,title", "--jq", ".[].title"))
	assert.Equal(t, "T\nU\n", stdout.String())

	stdout.Reset()
	require.NoError(t, runCmd(t, readyFactory(ios), "--json", "number,title"))
	assert.Equal(t, `[
  {
    "number": 3,
    "title": "T"
  },
  {
    "number": 4,
    "title": "U"
  }
]
`, stdout.String())
}

func TestNewCmdList_JSONErrors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "--json", "number,author")
	require.ErrorContains(t, err, "unknown JSON field: \"author\"\nAvailable fields:\n  number\n  title\n  state\n  labels\n  updated_at")

	err = runCmd(t, readyFactory(ios), "--jq", ".[].title")
	require.EqualError(t, err, "cannot use `--jq` without specifying `--json`")

	err = runCmd(t, readyFactory(ios), "--json", "title", "--jq", ".[")
	require.ErrorContains(t, err, "invalid --jq expression")
}

func TestNewCmdList_JSONTemplate(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuesPath), httpmock.StringResponse(http.StatusOK,
		`[{"number":3,"title":"T","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[]},
		  {"number":4,"title":"U","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[]}]`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "--json", "number", "--template", `{{range .}}{{.number}}{{"\n"}}{{end}}`))
	assert.Equal(t, "3\n4\n", stdout.String())

	err := runCmd(t, readyFactory(ios), "--template", "{{.}}")
	require.EqualError(t, err, "cannot use `--template` without specifying `--json`")

	err = runCmd(t, readyFactory(ios), "--json", "title", "--jq", ".", "--template", "{{.}}")
	require.EqualError(t, err, "cannot use `--jq` and `--template` together")

	err = runCmd(t, readyFactory(ios), "--json", "title", "--template", "{{range .}}")
	require.ErrorContains(t, err, "invalid --template")
}

func TestNewCmdList_Web(t *testing.T) {
	const listURL = "https://app.copia.io/my-org/my-repo/issues"
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "defaults", args: []string{"--web"}, wantURL: listURL + "?state=open"},
		{name: "short flag", args: []string{"-w"}, wantURL: listURL + "?state=open"},
		{name: "repo flag", args: []string{"-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/issues?state=open"},
		{name: "state", args: []string{"-w", "-s", "closed"}, wantURL: listURL + "?state=closed"},
		{name: "all states", args: []string{"-w", "--state", "all"}, wantURL: listURL + "?state=all"},
		{name: "search", args: []string{"-w", "-S", "plc timeout"}, wantURL: listURL + "?q=plc+timeout&state=open"},
		{name: "author login", args: []string{"-w", "-A", "jane"}, wantURL: listURL + "?poster=jane&state=open"},
		{name: "author me", args: []string{"-w", "--author", "@me"}, wantURL: listURL + "?state=open&type=created_by"},
		{name: "assignee me", args: []string{"-w", "-a", "@me"}, wantURL: listURL + "?state=open&type=assigned"},
		{
			name: "assignee and author me", args: []string{"-w", "-a", "@me", "-A", "@me"},
			wantURL: listURL + "?state=open&type=assigned",
			wantErr: "warning: --author is not applied in the browser\n",
		},
		{
			name: "assignee me and author login", args: []string{"-w", "-a", "@me", "-A", "jane", "-S", "x"},
			wantURL: listURL + "?poster=jane&q=x&state=open&type=assigned",
		},
		{
			name: "filters the web list takes by ID", args: []string{"-w", "-l", "bug", "-a", "jane", "--mention", "amy", "-m", "v1.0", "-L", "5"},
			wantURL: listURL + "?state=open",
			wantErr: "warning: --label is not applied in the browser\n" +
				"warning: --assignee is not applied in the browser\n" +
				"warning: --mention is not applied in the browser\n" +
				"warning: --milestone is not applied in the browser\n",
		},
		{
			name: "tty", args: []string{"-w"}, tty: true,
			wantURL: listURL + "?state=open",
			wantErr: "Opening app.copia.io/my-org/my-repo/issues?state=open in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_TOKEN", "")
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			repo := tt.repo
			if repo == "" {
				repo = "my-org/my-repo"
			}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: repo, Browser: b}

			require.NoError(t, runCmd(t, f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdList_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()

	b := &browsertest.Browser{}
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: b}
	require.EqualError(t, runCmd(t, f, "-w", "--state", "merged"), `invalid state "merged": valid values are {open|closed|all}`)
	assert.Empty(t, b.URLs)

	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: b,
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(t, noConfig, "-w"), "no config")

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}
	err := runCmd(t, noRepo, "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(t, failing, "-w"), "no browser")
	assert.Empty(t, reg.Requests)
}
