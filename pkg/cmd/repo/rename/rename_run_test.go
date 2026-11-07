package rename

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGit runs git with the current (test) environment and fails on error.
func runGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// isolateGit keeps the user's and the system's git config out of the test.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

// setupClone creates a bare repository at <parent>/my-org/new-repo.git, the
// renamed repository on the server, and a clone of it in a work directory,
// whose remotes are replaced by remotes (name to URL, added in order). It
// chdirs into the clone and returns parent and the clone's directory.
func setupClone(t *testing.T, remotes [][2]string) (string, string) {
	t.Helper()
	isolateGit(t)
	parent := t.TempDir()
	src := t.TempDir()
	runGit(t, "init", "-q", src)
	runGit(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, "clone", "-q", "--bare", src, filepath.Join(parent, "my-org", "new-repo.git"))

	work := filepath.Join(t.TempDir(), "work")
	runGit(t, "clone", "-q", filepath.Join(parent, "my-org", "new-repo.git"), work)
	runGit(t, "-C", work, "remote", "remove", "origin")
	for _, r := range remotes {
		runGit(t, "-C", work, "remote", "add", r[0], r[1])
	}
	t.Chdir(work)
	return parent, work
}

// reachServer rewrites the Copia URLs to the bare repositories under parent,
// so git can reach the remotes it finds after the rename.
func reachServer(t *testing.T, parent string) {
	t.Helper()
	base := "url.file://" + filepath.ToSlash(parent) + "/.insteadOf"
	t.Setenv("GIT_CONFIG_COUNT", "3")
	t.Setenv("GIT_CONFIG_KEY_0", base)
	t.Setenv("GIT_CONFIG_VALUE_0", "https://app.copia.io/")
	t.Setenv("GIT_CONFIG_KEY_1", base)
	t.Setenv("GIT_CONFIG_VALUE_1", "git@app.copia.io:")
	t.Setenv("GIT_CONFIG_KEY_2", base)
	t.Setenv("GIT_CONFIG_VALUE_2", "ssh://git@app.copia.io/")
}

// storedURL returns the raw URL of the remote name from the clone's config,
// not rewritten by insteadOf rules.
func storedURL(t *testing.T, dir, name string) string {
	t.Helper()
	return runGit(t, "-C", dir, "config", "--get", "remote."+name+".url")
}

func renameFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", BaseRepo: cmdutil.DetectBaseRepo()}
}

func TestNewCmdRename_UpdatesRemote(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		repo    string // the repository name in the URL, which the PATCH uses
		wantURL string
		reach   bool // the insteadOf rules of reachServer match the new URL
	}{
		{name: "https", url: "https://app.copia.io/my-org/my-repo.git", repo: "my-repo", wantURL: "https://app.copia.io/my-org/new-repo.git", reach: true},
		{name: "https without .git", url: "https://app.copia.io/my-org/my-repo", repo: "my-repo", wantURL: "https://app.copia.io/my-org/new-repo", reach: true},
		{name: "scp-like ssh", url: "git@app.copia.io:my-org/my-repo.git", repo: "my-repo", wantURL: "git@app.copia.io:my-org/new-repo.git", reach: true},
		{name: "ssh URL", url: "ssh://git@app.copia.io/my-org/my-repo.git", repo: "my-repo", wantURL: "ssh://git@app.copia.io/my-org/new-repo.git", reach: true},
		{name: "other case", url: "https://APP.copia.io/my-org/My-Repo.git", repo: "My-Repo", wantURL: "https://APP.copia.io/my-org/new-repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent, work := setupClone(t, [][2]string{{"origin", tt.url}})

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			var payload map[string]any
			reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/"+tt.repo), renamed(t, &payload))

			ios, _, stdout, stderr := iostreams.Test()
			require.NoError(t, runCmd(renameFactory(ios), "new-repo", "--yes"))

			assert.Equal(t, "Renamed repository my-org/new-repo\nUpdated the \"origin\" remote\n", stdout.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, tt.wantURL, storedURL(t, work, "origin"))

			if tt.reach {
				reachServer(t, parent)
				runGit(t, "ls-remote", "-q", "origin")
			}
		})
	}
}

func TestNewCmdRename_PicksRemote(t *testing.T) {
	_, work := setupClone(t, [][2]string{
		{"github", "https://github.com/my-org/my-repo.git"},
		{"origin", "https://app.copia.io/my-org/my-repo.git"},
		{"upstream", "https://app.copia.io/my-org/my-repo.git"},
	})
	runGit(t, "config", "remote.upstream."+cmdutil.RemoteResolvedKey, "base")

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	var payload map[string]any
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo"), renamed(t, &payload))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, runCmd(renameFactory(ios), "new-repo", "--yes"))

	assert.Equal(t, "Renamed repository my-org/new-repo\nUpdated the \"upstream\" remote\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "https://app.copia.io/my-org/new-repo.git", storedURL(t, work, "upstream"))
	assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", storedURL(t, work, "origin"))
	assert.Equal(t, "https://github.com/my-org/my-repo.git", storedURL(t, work, "github"))
}

func TestNewCmdRename_RepoFlagLeavesRemote(t *testing.T) {
	_, work := setupClone(t, [][2]string{{"origin", "https://app.copia.io/my-org/my-repo.git"}})

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	var payload map[string]any
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo"), renamed(t, &payload))

	ios, _, stdout, stderr := iostreams.Test()
	f := renameFactory(ios)
	f.Repo = "my-org/my-repo"
	require.NoError(t, runCmd(f, "new-repo", "--yes"))

	assert.Equal(t, "Renamed repository my-org/new-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", storedURL(t, work, "origin"))
}

func TestNewCmdRename_RemoteWarnings(t *testing.T) {
	tests := []struct {
		name       string
		remotes    [][2]string
		readOnly   bool
		wantStderr string
	}{
		{
			name:       "remote on another host",
			remotes:    [][2]string{{"origin", "https://github.com/my-org/my-repo.git"}},
			wantStderr: "! Warning: unable to update remote: no git remote points at my-org/my-repo on app.copia.io\n",
		},
		{
			name:       "config not writable",
			remotes:    [][2]string{{"origin", "https://app.copia.io/my-org/my-repo.git"}},
			readOnly:   true,
			wantStderr: "! Warning: unable to update remote \"origin\": ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, work := setupClone(t, tt.remotes)
			if tt.readOnly {
				gitDir := filepath.Join(work, ".git")
				require.NoError(t, os.Chmod(gitDir, 0o555))
				t.Cleanup(func() { _ = os.Chmod(gitDir, 0o755) })
				if f, err := os.CreateTemp(gitDir, "probe"); err == nil {
					_ = f.Close()
					t.Skip("the git directory stays writable (running as root)")
				}
			}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			var payload map[string]any
			reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo"), renamed(t, &payload))

			ios, _, stdout, stderr := iostreams.Test()
			require.NoError(t, runCmd(renameFactory(ios), "new-repo", "--yes"))

			assert.Equal(t, "Renamed repository my-org/new-repo\n", stdout.String())
			assert.True(t, strings.HasPrefix(stderr.String(), tt.wantStderr), "stderr: %q", stderr.String())
			assert.Equal(t, tt.remotes[0][1], storedURL(t, work, "origin"))
		})
	}
}

func TestNewCmdRename_NotInGitRepository(t *testing.T) {
	isolateGit(t)
	t.Chdir(t.TempDir())

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(renameFactory(ios), "new-repo", "--yes")
	require.ErrorContains(t, err, "could not determine repository")
	assert.Empty(t, stdout.String())
}
