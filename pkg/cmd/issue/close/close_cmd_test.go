package close

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
)

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdClose(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdClose_RunsWithComment(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var posted string
	reg.Register(httpmock.REST("POST", commentPath), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		posted = string(b)
		return httpmock.StringResponse(http.StatusCreated, `{"id":1}`)(req)
	})
	reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusOK, `{"number":12}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--comment", "bye")
	require.NoError(t, err)
	assert.JSONEq(t, `{"body":"bye"}`, posted)
	assert.Equal(t, "Closed issue #12\n", stdout.String())
	assert.Equal(t, "token test-token", reg.Requests[1].Header.Get("Authorization"))
}

func TestNewCmdClose_InvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "abc")
	require.EqualError(t, err, "invalid issue number: abc")
}

func TestNewCmdClose_ArgCount(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios))
	require.Error(t, err)
}

func TestNewCmdClose_AuthFailure(t *testing.T) {
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

func TestNewCmdClose_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

func runOpts(reg *httpmock.Registry, comment string) (*CloseOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &CloseOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Comment:    comment,
	}, stdout
}

func failing(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

func TestCloseRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name:    "comment rejected",
			comment: "bye",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", commentPath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to add comment (HTTP 403)",
		},
		{
			name:    "comment transport error",
			comment: "bye",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", commentPath), failing)
			},
			wantErr: "boom",
		},
		{
			name: "close rejected",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "failed to close issue (HTTP 404)",
		},
		{
			name: "close transport error",
			stubs: func(reg *httpmock.Registry) {
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

			opts, stdout := runOpts(reg, tt.comment)
			err := CloseRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestCloseRun_AcceptsCreatedStatus(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusCreated, `{}`))

	opts, stdout := runOpts(reg, "")
	require.NoError(t, CloseRun(opts))
	assert.Equal(t, "Closed issue #12\n", stdout.String())
}

func TestCloseRun_InvalidHost(t *testing.T) {
	for _, comment := range []string{"", "bye"} {
		reg := &httpmock.Registry{}
		opts, _ := runOpts(reg, comment)
		opts.Host = "bad\x7fhost"
		err := CloseRun(opts)
		require.Error(t, err)
		assert.Empty(t, reg.Requests)
	}
}
