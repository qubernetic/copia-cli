package list

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
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
