package archive

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoPath = "/api/v1/repos/my-org/my-repo"

// patched answers the PATCH as Gitea does and stores its JSON payload in
// into.
func patched(t *testing.T, into *map[string]any) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, into))
		return httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo"}`)(req)
	}
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdArchive(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdArchive(t *testing.T) {
	tests := []struct {
		name string
		args []string
		repo string
	}{
		{name: "argument", args: []string{"my-org/my-repo", "--yes"}},
		{name: "current repository", args: []string{"-y"}, repo: "my-org/my-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			var payload map[string]any
			reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo","archived":false}`))
			reg.Register(httpmock.REST("PATCH", repoPath), patched(t, &payload))

			ios, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: tt.repo}

			require.NoError(t, runCmd(f, tt.args...))
			assert.Equal(t, map[string]any{"archived": true}, payload)
			assert.Equal(t, "Archived repository my-org/my-repo\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestNewCmdArchive_AlreadyArchived(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo","archived":true}`))

	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	require.NoError(t, runCmd(f, "my-org/my-repo", "--yes"))
	assert.Empty(t, stdout.String())
	assert.Equal(t, "! Repository my-org/my-repo is already archived\n", stderr.String())
}

func TestNewCmdArchive_Refused(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no --yes", args: []string{"my-org/my-repo"}, wantErr: "--yes required when not running interactively"},
		{name: "no owner", args: []string{"my-repo", "--yes"}, wantErr: `expected owner/repo format, got "my-repo"`},
		{name: "two arguments", args: []string{"my-org/a", "my-org/b", "--yes"}, wantErr: "accepts at most 1 arg(s), received 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

			require.EqualError(t, runCmd(f, tt.args...), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRun_HTTPError(t *testing.T) {
	tests := []struct {
		name     string
		getCode  int
		getBody  string
		patch    bool
		patchErr int
		body     string
		wantErr  string
	}{
		{name: "not found", getCode: http.StatusNotFound, getBody: `{"message":"The target couldn't be found."}`, wantErr: "repository my-org/my-repo not found"},
		{name: "get fails", getCode: http.StatusInternalServerError, wantErr: "failed to get repository (HTTP 500)"},
		{name: "bad json", getCode: http.StatusOK, getBody: `{`, wantErr: "parsing response: unexpected end of JSON input"},
		{name: "patch refused", getCode: http.StatusOK, getBody: `{"archived":false}`, patch: true, patchErr: http.StatusForbidden, body: `{"message":"forbidden"}`, wantErr: "failed to archive repository (HTTP 403): forbidden"},
		{name: "patch no message", getCode: http.StatusOK, getBody: `{"archived":false}`, patch: true, patchErr: http.StatusInternalServerError, wantErr: "failed to archive repository (HTTP 500)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(tt.getCode, tt.getBody))
			if tt.patch {
				reg.Register(httpmock.REST("PATCH", repoPath), httpmock.StringResponse(tt.patchErr, tt.body))
			}

			ios, _, stdout, _ := iostreams.Test()
			err := Run(&Options{
				IO: ios, HTTPClient: cmdutil.NewHTTPClient(),
				Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo",
				Confirmed: true, Archived: true,
			})
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRun_Unarchive(t *testing.T) {
	tests := []struct {
		name       string
		archived   bool
		wantPatch  bool
		wantStdout string
		wantStderr string
	}{
		{name: "archived", archived: true, wantPatch: true, wantStdout: "Unarchived repository my-org/my-repo\n"},
		{name: "not archived", archived: false, wantStderr: "! Repository my-org/my-repo is not archived\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			body, err := json.Marshal(map[string]any{"archived": tt.archived})
			require.NoError(t, err)
			reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(http.StatusOK, string(body)))
			var payload map[string]any
			if tt.wantPatch {
				reg.Register(httpmock.REST("PATCH", repoPath), patched(t, &payload))
			}

			ios, _, stdout, stderr := iostreams.Test()
			require.NoError(t, Run(&Options{
				IO: ios, HTTPClient: cmdutil.NewHTTPClient(),
				Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo",
				Confirmed: true, Archived: false,
			}))
			if tt.wantPatch {
				assert.Equal(t, map[string]any{"archived": false}, payload)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
