package unpin

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

func runOpts(reg *httpmock.Registry) (*UnpinOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &UnpinOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
	}, stdout, stderr
}

func TestUnpinRun(t *testing.T) {
	tests := []struct {
		name       string
		issue      string
		unpin      httpmock.Responder
		wantOut    string
		wantErrOut string
		wantErr    string
	}{
		{
			name:    "unpins a pinned issue",
			issue:   `{"title":"Broken","pin_order":3}`,
			unpin:   httpmock.StringResponse(http.StatusNoContent, ""),
			wantOut: "Unpinned issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:    "unpins when the server sends no pin_order",
			issue:   `{"title":"Broken"}`,
			unpin:   httpmock.StringResponse(http.StatusNoContent, ""),
			wantOut: "Unpinned issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:       "not pinned sends nothing",
			issue:      `{"title":"Broken","pin_order":0}`,
			wantErrOut: "! Issue my-org/my-repo#12 (Broken) is not pinned\n",
		},
		{
			name:    "forbidden",
			issue:   `{"title":"Broken","pin_order":3}`,
			unpin:   httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: "cannot unpin issue my-org/my-repo#12: permission denied (HTTP 403); unpinning an issue needs write access to the repository's issues",
		},
		{
			name:    "unpin endpoint not served",
			issue:   `{"title":"Broken","pin_order":3}`,
			unpin:   httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "app.copia.io does not support unpinning issues (HTTP 404 on DELETE /repos/my-org/my-repo/issues/12/pin)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, tt.issue))
			if tt.unpin != nil {
				reg.Register(httpmock.REST("DELETE", issuePath+"/pin"), tt.unpin)
			}

			opts, stdout, stderr := runOpts(reg)
			err := UnpinRun(opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}

func TestUnpinRun_IssueNotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusNotFound, `{}`))

	opts, stdout, _ := runOpts(reg)
	require.EqualError(t, UnpinRun(opts), "issue my-org/my-repo#12 not found")
	assert.Empty(t, stdout.String())
	assert.Len(t, reg.Requests, 1, "no unpin request for a missing issue")
}

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdUnpin(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdUnpin_Number(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{"title":"Broken","pin_order":3}`))
	reg.Register(httpmock.REST("DELETE", issuePath+"/pin"), httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "#12"))
	assert.Equal(t, "Unpinned issue my-org/my-repo#12 (Broken)\n", stdout.String())
}

func TestNewCmdUnpin_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/thing/issues/5"),
		httpmock.StringResponse(http.StatusOK, `{"title":"Elsewhere","pin_order":3}`))
	reg.Register(httpmock.REST("DELETE", "/api/v1/repos/other/thing/issues/5/pin"),
		httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "https://app.copia.io/other/thing/issues/5"))
	assert.Equal(t, "Unpinned issue other/thing#5 (Elsewhere)\n", stdout.String())
}

func TestNewCmdUnpin_URLOnOtherHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "https://copia.example.com/other/thing/issues/5")
	require.EqualError(t, err, "issue URL host copia.example.com does not match the active host app.copia.io")
}

func TestNewCmdUnpin_InvalidArgument(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, readyFactory(ios), "abc"), "invalid issue number or URL: abc")
	require.Error(t, runCmd(t, readyFactory(ios)))
}

func TestNewCmdUnpin_AuthFailure(t *testing.T) {
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

func TestNewCmdUnpin_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
