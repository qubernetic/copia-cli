package rename

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

// renamed answers the PATCH as Gitea does and stores its JSON payload in
// into.
func renamed(t *testing.T, into *map[string]any) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, into))
		return httpmock.StringResponse(http.StatusOK, `{"name":"new-repo","full_name":"my-org/new-repo","owner":{"login":"my-org"}}`)(req)
	}
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdRename(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdRename_RepoFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var payload map[string]any
	reg.Register(httpmock.REST("PATCH", repoPath), renamed(t, &payload))

	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	require.NoError(t, runCmd(f, "new-repo", "--yes"))
	assert.Equal(t, map[string]any{"name": "new-repo"}, payload)
	assert.Equal(t, "Renamed repository my-org/new-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNewCmdRename_Refused(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "no --yes",
			args:    []string{"new-repo"},
			wantErr: "renaming my-org/my-repo to new-repo needs confirmation; use --yes to confirm",
		},
		{
			name:    "slash in the name",
			args:    []string{"other-org/new-repo", "--yes"},
			wantErr: "New repository name cannot contain '/' character - to transfer a repository to a new owner, see <https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository>.",
		},
		{
			name:    "no name",
			args:    []string{"--yes"},
			wantErr: "accepts 1 arg(s), received 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

			err := runCmd(f, tt.args...)
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRenameRun_HTTPError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "name taken", status: http.StatusUnprocessableEntity, body: `{"message":"repository already exists [uname: my-org, name: new-repo]"}`, wantErr: "failed to rename repository (HTTP 422): repository already exists [uname: my-org, name: new-repo]"},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"The target couldn't be found."}`, wantErr: "failed to rename repository (HTTP 404): The target couldn't be found."},
		{name: "no message", status: http.StatusForbidden, body: ``, wantErr: "failed to rename repository (HTTP 403)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("PATCH", repoPath), httpmock.StringResponse(tt.status, tt.body))

			ios, _, stdout, _ := iostreams.Test()
			err := RenameRun(&RenameOptions{
				IO: ios, HTTPClient: cmdutil.NewHTTPClient(),
				Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo",
				NewName: "new-repo", Confirmed: true,
			})
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRenameURL(t *testing.T) {
	tests := []struct {
		raw    string
		want   string
		wantOK bool
	}{
		{raw: "https://app.copia.io/my-org/my-repo.git", want: "https://app.copia.io/my-org/new-repo.git", wantOK: true},
		{raw: "https://app.copia.io/my-org/my-repo/", want: "https://app.copia.io/my-org/new-repo", wantOK: true},
		{raw: "https://app.copia.io/my-org/my-repo.GIT", want: "https://app.copia.io/my-org/new-repo.GIT", wantOK: true},
		{raw: "git@app.copia.io:my-repo.git", want: "git@app.copia.io:new-repo.git", wantOK: true},
		{raw: "https://app.copia.io/my-org/other.git"},
		{raw: "my-repo"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := renameURL(tt.raw, "my-repo", "new-repo")
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSameHostName(t *testing.T) {
	tests := []struct {
		remote     string
		configured string
		want       bool
	}{
		{remote: "https://app.copia.io/o/r", configured: "app.copia.io", want: true},
		{remote: "https://App.Copia.io:8443/o/r", configured: "app.copia.io", want: true},
		{remote: "ssh://git@app.copia.io:2222/o/r", configured: "app.copia.io:443", want: true},
		{remote: "git@app.copia.io:o/r.git", configured: "app.copia.io", want: true},
		{remote: "app.copia.io:o/r.git", configured: "app.copia.io", want: true},
		{remote: "https://github.com/o/r", configured: "app.copia.io"},
		{remote: "/srv/git/o/r.git", configured: "app.copia.io"},
		{remote: "https://%zz/o/r", configured: "app.copia.io"},
		{remote: "https://app.copia.io/o/r", configured: "app.copia.io/path"},
	}
	for _, tt := range tests {
		t.Run(tt.remote+" "+tt.configured, func(t *testing.T) {
			assert.Equal(t, tt.want, sameHostName(remoteHost(tt.remote), tt.configured))
		})
	}
}
