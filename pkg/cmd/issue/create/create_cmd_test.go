package create

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
	cmd := NewCmdCreate(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdCreate_RunsWithFlags(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"},{"id":2,"name":"plc"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[]`))
	var posted string
	reg.Register(httpmock.REST("POST", issuesPath), recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdIssue)))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "--title", "Fix sensor mapping", "-b", "details", "-l", "bug,plc")
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Fix sensor mapping","body":"details","labels":[1,2]}`, posted)
	assert.Equal(t, "Created issue #13: Fix sensor mapping\nhttps://app.copia.io/my-org/my-repo/issues/13\n", stdout.String())
}

func TestNewCmdCreate_AssigneeAndMilestoneFlags(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/1"), httpmock.StringResponse(http.StatusOK, `{"id":1,"title":"Sprint 1"}`))
	var posted string
	reg.Register(httpmock.REST("POST", issuesPath), recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdIssue)))

	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "--title", "Fix sensor mapping", "-a", "john", "--assignee", "jane", "-m", "1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Fix sensor mapping","assignees":["john","jane"],"milestone":1}`, posted)
}

func TestNewCmdCreate_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runCmd(t, f, "--title", "x")
	require.EqualError(t, err, "no config")
}

func TestNewCmdCreate_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "--title", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

func TestNewCmdCreate_MissingTitle(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "--body", "x")
	require.EqualError(t, err, "title required")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func runOpts(reg *httpmock.Registry) (*CreateOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &CreateOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Title:      "Fix sensor mapping",
	}, stdout
}

func TestCreateRun_Errors(t *testing.T) {
	tests := []struct {
		name      string
		responder httpmock.Responder
		wantErr   string
	}{
		{
			name:      "server rejects",
			responder: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"bad title"}`),
			wantErr:   `failed to create issue (HTTP 422): {"message":"bad title"}`,
		},
		{
			name:      "transport error",
			responder: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr:   "connecting to app.copia.io:",
		},
		{
			name:      "malformed JSON",
			responder: httpmock.StringResponse(http.StatusCreated, `{not json`),
			wantErr:   "invalid character",
		},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(errReader{}), Header: http.Header{}}, nil
			},
			wantErr: "read failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("POST", issuesPath), tt.responder)

			opts, stdout := runOpts(reg)
			err := CreateRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestCreateRun_LabelLookupFails(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusInternalServerError, `{}`))

	opts, _ := runOpts(reg)
	opts.Labels = []string{"bug"}
	err := CreateRun(opts)
	require.EqualError(t, err, "API error (HTTP 500)")
	assert.Len(t, reg.Requests, 1, "no issue is created")
}

func TestCreateRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _ := runOpts(reg)
	opts.Host = "bad\x7fhost"
	require.Error(t, CreateRun(opts))
	assert.Empty(t, reg.Requests)
}
