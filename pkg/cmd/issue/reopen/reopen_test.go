package reopen

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

const (
	issuePath   = "/api/v1/repos/my-org/my-repo/issues/12"
	commentPath = "/api/v1/repos/my-org/my-repo/issues/12/comments"
	closedIssue = `{"number":12,"title":"Broken build","state":"closed"}`
	openIssue   = `{"number":12,"title":"Broken build","state":"open"}`
)

func runOpts(reg *httpmock.Registry, comment string) (*ReopenOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &ReopenOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Comment:    comment,
	}, stdout, stderr
}

func failing(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

func TestReopenRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	var patched string
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
	reg.Register(httpmock.REST("PATCH", issuePath), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		patched = string(b)
		return httpmock.StringResponse(http.StatusCreated, openIssue)(req)
	})

	opts, stdout, stderr := runOpts(reg, "")
	require.NoError(t, ReopenRun(opts))
	assert.JSONEq(t, `{"state":"open"}`, patched)
	assert.Equal(t, "Reopened issue #12\n", stdout.String())
	assert.Empty(t, stderr.String())
	for _, r := range reg.Requests {
		assert.Equal(t, "token test-token", r.Header.Get("Authorization"))
	}
}

func TestReopenRun_AlreadyOpen(t *testing.T) {
	for _, comment := range []string{"", "back again"} {
		reg := &httpmock.Registry{}
		reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, openIssue))

		opts, stdout, stderr := runOpts(reg, comment)
		require.NoError(t, ReopenRun(opts))
		reg.Verify(t)
		assert.Len(t, reg.Requests, 1, "no comment and no PATCH for an open issue")
		assert.Empty(t, stdout.String())
		assert.Equal(t, "! Issue my-org/my-repo#12 (Broken build) is already open\n", stderr.String())
	}
}

func TestReopenRun_WithComment(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	var posted string
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
	reg.Register(httpmock.REST("POST", commentPath), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		posted = string(b)
		return httpmock.StringResponse(http.StatusCreated, `{"id":1}`)(req)
	})
	reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusCreated, openIssue))

	opts, stdout, _ := runOpts(reg, "Still failing")
	require.NoError(t, ReopenRun(opts))
	assert.JSONEq(t, `{"body":"Still failing"}`, posted)
	require.Len(t, reg.Requests, 3)
	assert.Equal(t, "POST", reg.Requests[1].Method, "comment before reopen")
	assert.Equal(t, "PATCH", reg.Requests[2].Method)
	assert.Equal(t, "Reopened issue #12\n", stdout.String())
}

func TestReopenRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name: "issue not found",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "issue #12 not found",
		},
		{
			name: "lookup rejected",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to read issue (HTTP 403)",
		},
		{
			name: "lookup transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), failing)
			},
			wantErr: "boom",
		},
		{
			name: "lookup bad json",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{`))
			},
			wantErr: "unexpected end of JSON input",
		},
		{
			name:    "comment rejected",
			comment: "bye",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
				reg.Register(httpmock.REST("POST", commentPath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to add comment (HTTP 403)",
		},
		{
			name:    "comment transport error",
			comment: "bye",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
				reg.Register(httpmock.REST("POST", commentPath), failing)
			},
			wantErr: "boom",
		},
		{
			name: "reopen rejected",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
				reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "failed to reopen issue (HTTP 404)",
		},
		{
			name: "reopen transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
				reg.Register(httpmock.REST("PATCH", issuePath), failing)
			},
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)

			opts, stdout, _ := runOpts(reg, tt.comment)
			err := ReopenRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestReopenRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _, _ := runOpts(reg, "")
	opts.Host = "bad\x7fhost"
	require.Error(t, ReopenRun(opts))
	assert.Empty(t, reg.Requests)
}

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdReopen(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdReopen_RunsWithComment(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, closedIssue))
	reg.Register(httpmock.REST("POST", commentPath), httpmock.StringResponse(http.StatusCreated, `{"id":1}`))
	reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusCreated, openIssue))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "12", "-c", "again"))
	assert.Equal(t, "Reopened issue #12\n", stdout.String())
}

func TestNewCmdReopen_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/thing/issues/5"),
		httpmock.StringResponse(http.StatusOK, `{"number":5,"title":"x","state":"closed"}`))
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/other/thing/issues/5"),
		httpmock.StringResponse(http.StatusCreated, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "https://app.copia.io/other/thing/issues/5"))
	assert.Equal(t, "Reopened issue #5\n", stdout.String())
}

func TestNewCmdReopen_URLOnOtherHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "https://copia.example.com/other/thing/issues/5")
	require.EqualError(t, err, "issue URL host copia.example.com does not match the active host app.copia.io")
}

func TestNewCmdReopen_InvalidArgument(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, readyFactory(ios), "abc"), "invalid issue number or URL: abc")
	require.Error(t, runCmd(t, readyFactory(ios)))
}

func TestNewCmdReopen_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	require.EqualError(t, runCmd(t, f, "12"), "no config")
}

func TestNewCmdReopen_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
