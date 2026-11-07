package sync

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

const forkPath = "/api/v1/repos/my-user/my-fork"

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdSync(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func testFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", BaseRepo: cmdutil.DetectBaseRepo()}
}

// mergeUpstream answers the merge-upstream POST with status and body and
// stores its JSON payload in into.
func mergeUpstream(t *testing.T, into *map[string]any, status int, body string) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, into))
		return httpmock.WithHeader(httpmock.StringResponse(status, body), "Content-Type", "application/json;charset=utf-8")(req)
	}
}

func TestNewCmdSync_Remote(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		status      int
		body        string
		wantPayload map[string]any
		wantOut     string
		wantErr     string
	}{
		{
			name:        "fast-forward",
			args:        []string{"my-user/my-fork", "--branch", "dev"},
			status:      http.StatusOK,
			body:        `{"merge_type":"fast-forward"}`,
			wantPayload: map[string]any{"branch": "dev", "ff_only": true},
			wantOut:     "Synced the \"my-user:dev\" branch from its parent\n",
		},
		{
			name:        "up to date",
			args:        []string{"my-user/my-fork", "-b", "dev"},
			status:      http.StatusOK,
			body:        `{"merge_type":"up-to-date"}`,
			wantPayload: map[string]any{"branch": "dev", "ff_only": true},
			wantOut:     "The \"my-user:dev\" branch is already up to date with its parent\n",
		},
		{
			name:        "diverged",
			args:        []string{"my-user/my-fork", "-b", "dev"},
			status:      http.StatusBadRequest,
			body:        `{"message":"fast-forward merge not possible: branch has diverged","url":"https://app.copia.io/api/swagger"}`,
			wantPayload: map[string]any{"branch": "dev", "ff_only": true},
			wantErr:     "can't sync because there are diverging changes; run copia-cli repo sync --force in a local clone and push",
		},
		{
			name:        "conflict",
			args:        []string{"my-user/my-fork", "-b", "dev"},
			status:      http.StatusConflict,
			body:        `{"message":"merge conflicts","url":"https://app.copia.io/api/swagger"}`,
			wantPayload: map[string]any{"branch": "dev", "ff_only": true},
			wantErr:     "can't sync because there are diverging changes; run copia-cli repo sync --force in a local clone and push",
		},
		{
			name:        "not a fork",
			args:        []string{"my-user/my-fork", "-b", "dev"},
			status:      http.StatusBadRequest,
			body:        `{"message":"repo is not a fork","url":"https://app.copia.io/api/swagger"}`,
			wantPayload: map[string]any{"branch": "dev", "ff_only": true},
			wantErr:     "my-user/my-fork is not a fork",
		},
		{
			name:        "other error",
			args:        []string{"my-user/my-fork", "-b", "dev"},
			status:      http.StatusForbidden,
			body:        `{"message":"permission denied","url":"https://app.copia.io/api/swagger"}`,
			wantPayload: map[string]any{"branch": "dev", "ff_only": true},
			wantErr:     "failed to sync my-user/my-fork (HTTP 403): permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			var payload map[string]any
			reg.Register(httpmock.REST("POST", forkPath+"/merge-upstream"), mergeUpstream(t, &payload, tt.status, tt.body))

			ios, _, stdout, stderr := iostreams.Test()
			err := runCmd(testFactory(ios), tt.args...)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantPayload, payload)
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestNewCmdSync_RemoteDefaultBranch(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", forkPath), httpmock.StringResponse(http.StatusOK, `{"full_name":"my-user/my-fork","default_branch":"trunk"}`))
	var payload map[string]any
	reg.Register(httpmock.REST("POST", forkPath+"/merge-upstream"), mergeUpstream(t, &payload, http.StatusOK, `{"merge_type":"fast-forward"}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "my-user/my-fork"))
	assert.Equal(t, map[string]any{"branch": "trunk", "ff_only": true}, payload)
	assert.Equal(t, "Synced the \"my-user:trunk\" branch from its parent\n", stdout.String())
}

func TestNewCmdSync_RemoteRouteMissing(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", forkPath+"/merge-upstream"),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusNotFound, "404 page not found\n"), "Content-Type", "text/plain; charset=utf-8"))

	ios, _, stdout, stderr := iostreams.Test()
	err := runCmd(testFactory(ios), "my-user/my-fork", "-b", "main")
	require.EqualError(t, err, "Copia does not offer fork sync through its API")
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNewCmdSync_RemoteRepositoryNotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", forkPath),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusNotFound, `{"message":"The target couldn't be found.","url":"https://app.copia.io/api/swagger"}`), "Content-Type", "application/json;charset=utf-8"))

	ios, _, _, _ := iostreams.Test()
	err := runCmd(testFactory(ios), "my-user/my-fork")
	require.EqualError(t, err, "failed to get repository my-user/my-fork (HTTP 404): The target couldn't be found.")
}

func TestNewCmdSync_RemoteRefused(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "--force",
			args:    []string{"my-user/my-fork", "--force"},
			wantErr: "--force is not supported with a repository argument; run copia-cli repo sync --force in a local clone and push",
		},
		{
			name:    "--source",
			args:    []string{"my-user/my-fork", "--source", "my-org/my-repo"},
			wantErr: "--source is not supported with a repository argument; a remote repository syncs from its parent",
		},
		{
			name:    "not owner/repo",
			args:    []string{"my-fork"},
			wantErr: `expected owner/repo format, got "my-fork"`,
		},
		{
			name:    "two arguments",
			args:    []string{"my-user/my-fork", "my-org/my-repo"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			err := runCmd(testFactory(ios), tt.args...)
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
