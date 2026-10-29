package view

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

const issuePath = "/api/v1/repos/my-org/my-repo/issues/12"

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdView(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdView_Runs(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK,
		`{"number":12,"title":"Fix PLC","state":"open","html_url":"https://app.copia.io/my-org/my-repo/issues/12","user":{"login":"john"},"comments":1}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12")
	require.NoError(t, err)
	assert.Equal(t, "#12 Fix PLC\nState: open  Author: john  Comments: 1\n\nhttps://app.copia.io/my-org/my-repo/issues/12\n", stdout.String())
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdView_JSONFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{"number":12,"title":"Fix PLC"}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--json", "number,title")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"title": "Fix PLC"`)
}

func TestNewCmdView_InvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "abc")
	require.EqualError(t, err, "invalid issue number: abc")
}

func TestNewCmdView_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runCmd(t, f, "12")
	require.EqualError(t, err, "no config")
}

func TestNewCmdView_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func viewOpts(reg *httpmock.Registry) (*ViewOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
	}, stdout
}

func TestViewRun_Errors(t *testing.T) {
	tests := []struct {
		name      string
		responder httpmock.Responder
		wantErr   string
	}{
		{"server error", httpmock.StringResponse(http.StatusInternalServerError, `{}`), "API error (HTTP 500)"},
		{"malformed JSON", httpmock.StringResponse(http.StatusOK, `{not json`), "invalid character"},
		{"transport error", func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }, "connecting to app.copia.io:"},
		{"body read error", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{}), Header: http.Header{}}, nil
		}, "read failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), tt.responder)

			opts, stdout := viewOpts(reg)
			err := ViewRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestViewRun_MultipleLabelsAndBody(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK,
		`{"number":12,"title":"Fix PLC","body":"Details","state":"closed","html_url":"u","user":{"login":"jane"},"labels":[{"name":"bug"},{"name":"plc"}],"comments":0}`))

	opts, stdout := viewOpts(reg)
	require.NoError(t, ViewRun(opts))
	assert.Equal(t, "#12 Fix PLC\nState: closed  Author: jane  Comments: 0\nLabels: bug, plc\n\nDetails\n\nu\n", stdout.String())
}

func TestViewRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _ := viewOpts(reg)
	opts.Host = "bad\x7fhost"
	require.Error(t, ViewRun(opts))
	assert.Empty(t, reg.Requests)
}

const commentsPath = "/api/v1/repos/my-org/my-repo/issues/12/comments"

func registerIssueWithBody(reg *httpmock.Registry) {
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK,
		`{"number":12,"title":"Fix PLC","body":"Details","state":"open","html_url":"https://app.copia.io/my-org/my-repo/issues/12","user":{"login":"john"},"comments":2}`))
}

func TestNewCmdView_Comments(t *testing.T) {
	for _, flag := range []string{"--comments", "-c"} {
		t.Run(flag, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			registerIssueWithBody(reg)
			reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusOK,
				`[{"body":"First","user":{"login":"jane"},"created_at":"2026-03-30T10:00:00Z"},`+
					`{"body":"Second\nline","user":{"login":"john"},"created_at":"2026-03-31T09:30:00Z"}]`))

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runCmd(t, readyFactory(ios), "12", flag))
			assert.Equal(t, "#12 Fix PLC\nState: open  Author: john  Comments: 2\n"+
				"\nDetails\n"+
				"\njane commented 2026-03-30T10:00:00Z:\nFirst\n"+
				"\njohn commented 2026-03-31T09:30:00Z:\nSecond\nline\n"+
				"\nhttps://app.copia.io/my-org/my-repo/issues/12\n", stdout.String())
			require.Len(t, reg.Requests, 2)
			assert.Equal(t, "token test-token", reg.Requests[1].Header.Get("Authorization"))
		})
	}
}

func TestNewCmdView_NoCommentsRequestWithoutFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerIssueWithBody(reg)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "12"))
	assert.Equal(t, "#12 Fix PLC\nState: open  Author: john  Comments: 2\n\nDetails\n\nhttps://app.copia.io/my-org/my-repo/issues/12\n", stdout.String())
	assert.Len(t, reg.Requests, 1)
}

func TestNewCmdView_ZeroComments(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerIssueWithBody(reg)
	reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "12", "--comments"))
	assert.Equal(t, "#12 Fix PLC\nState: open  Author: john  Comments: 2\n\nDetails\n\nhttps://app.copia.io/my-org/my-repo/issues/12\n", stdout.String())
}

func TestNewCmdView_CommentsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerIssueWithBody(reg)
	reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusInternalServerError, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--comments")
	require.EqualError(t, err, "API error (HTTP 500)")
	assert.Empty(t, stdout.String())
}

func TestNewCmdView_CommentsWithJSON(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--comments", "--json", "number")
	require.EqualError(t, err, "specify only one of --comments or --json")
	assert.Empty(t, reg.Requests)
}

func TestNewCmdView_JSONFieldsAndJQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK,
		`{"number":12,"title":"Bug","state":"open","body":"b","html_url":"https://x/12","user":{"login":"amy"},"labels":[],"comments":2}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "12", "--json", "user,html_url,comments", "--jq", ".user.login, .html_url, .comments"))
	assert.Equal(t, "amy\nhttps://x/12\n2\n", stdout.String())
}
