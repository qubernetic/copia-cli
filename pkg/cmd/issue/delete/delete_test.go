package delete

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

func runOpts(reg *httpmock.Registry, confirmed bool) (*DeleteOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &DeleteOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Confirmed:  confirmed,
	}, stdout, stderr
}

func TestDeleteRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("DELETE", issuePath), httpmock.StringResponse(http.StatusNoContent, ""))

	opts, stdout, stderr := runOpts(reg, true)
	require.NoError(t, DeleteRun(opts))
	assert.Equal(t, "Deleted issue my-org/my-repo#12\n", stdout.String())
	assert.Empty(t, stderr.String())
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestDeleteRun_NotConfirmed(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, stdout, _ := runOpts(reg, false)
	err := DeleteRun(opts)
	require.EqualError(t, err, "deleting issue my-org/my-repo#12 is irreversible; use --yes to confirm")
	assert.Empty(t, reg.Requests, "no request without --yes")
	assert.Empty(t, stdout.String())
}

func TestDeleteRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "forbidden",
			respond: httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s)"}`),
			wantErr: "cannot delete issue my-org/my-repo#12: permission denied (HTTP 403); deleting an issue needs admin rights on the repository",
		},
		{
			name:    "not found",
			respond: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "issue my-org/my-repo#12 not found",
		},
		{
			name:    "method not allowed",
			respond: httpmock.StringResponse(http.StatusMethodNotAllowed, ""),
			wantErr: "app.copia.io does not allow deleting issues (HTTP 405); close the issue with `copia-cli issue close` instead",
		},
		{
			name:    "other status",
			respond: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "failed to delete issue (HTTP 500)",
		},
		{
			name:    "transport error",
			respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("DELETE", issuePath), tt.respond)

			opts, stdout, _ := runOpts(reg, true)
			err := DeleteRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestDeleteRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _, _ := runOpts(reg, true)
	opts.Host = "bad\x7fhost"
	require.Error(t, DeleteRun(opts))
	assert.Empty(t, reg.Requests)
}

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdDelete(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdDelete_Yes(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("DELETE", issuePath), httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "12", "--yes"))
	assert.Equal(t, "Deleted issue my-org/my-repo#12\n", stdout.String())
}

func TestNewCmdDelete_WithoutYes(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12")
	require.EqualError(t, err, "deleting issue my-org/my-repo#12 is irreversible; use --yes to confirm")
	assert.Empty(t, reg.Requests)
}

func TestNewCmdDelete_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("DELETE", "/api/v1/repos/other/thing/issues/5"),
		httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "https://app.copia.io/other/thing/issues/5", "--yes"))
	assert.Equal(t, "Deleted issue other/thing#5\n", stdout.String())
}

func TestNewCmdDelete_URLOnOtherHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "https://copia.example.com/other/thing/issues/5", "--yes")
	require.EqualError(t, err, "issue URL host copia.example.com does not match the active host app.copia.io")
}

func TestNewCmdDelete_InvalidArgument(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, readyFactory(ios), "abc", "--yes"), "invalid issue number or URL: abc")
	require.Error(t, runCmd(t, readyFactory(ios)))
}

func TestNewCmdDelete_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	require.EqualError(t, runCmd(t, f, "12", "--yes"), "no config")
}

func TestNewCmdDelete_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12", "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
