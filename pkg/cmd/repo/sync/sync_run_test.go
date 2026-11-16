package sync

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS == "windows" {
		t.Skip("the ssh stub is a shell script")
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

// server holds bare repositories under dir, reached through scp-like
// remotes git@app.copia.io:<owner>/<repo>.git, which serveSSH routes to dir.
type server struct {
	dir string
}

// newServer creates an empty server and points git's ssh at a stub that
// runs the requested git command in the server's directory.
func newServer(t *testing.T) *server {
	t.Helper()
	isolateGit(t)
	s := &server{dir: t.TempDir()}
	stub := filepath.Join(t.TempDir(), "ssh")
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\ncd \"$SYNC_TEST_SERVER\" && eval \"$2\"\n"), 0o755))
	t.Setenv("SYNC_TEST_SERVER", s.dir)
	t.Setenv("GIT_SSH_COMMAND", stub)
	t.Setenv("GIT_SSH_VARIANT", "simple")
	return s
}

// url returns the scp-like remote URL of name ("owner/repo").
func (s *server) url(name string) string {
	return "git@app.copia.io:" + name + ".git"
}

// create makes the bare repository name with one commit on main.
func (s *server) create(t *testing.T, name string) {
	t.Helper()
	src := t.TempDir()
	runGit(t, "init", "-q", "-b", "main", src)
	runGit(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, "clone", "-q", "--bare", src, filepath.Join(s.dir, name+".git"))
}

// commit adds an empty commit on branch of the repository name, creating the
// branch from main when it does not exist, and returns its SHA.
func (s *server) commit(t *testing.T, name, branch, message string) string {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "tmp")
	runGit(t, "clone", "-q", filepath.Join(s.dir, name+".git"), tmp)
	if branch != "main" {
		runGit(t, "-C", tmp, "checkout", "-q", "-B", branch)
	}
	runGit(t, "-C", tmp, "commit", "-q", "--allow-empty", "-m", message)
	runGit(t, "-C", tmp, "push", "-q", "origin", branch)
	return runGit(t, "-C", tmp, "rev-parse", "HEAD")
}

// head returns the SHA of branch of the repository name.
func (s *server) head(t *testing.T, name, branch string) string {
	t.Helper()
	return runGit(t, "-C", filepath.Join(s.dir, name+".git"), "rev-parse", "refs/heads/"+branch)
}

// clone clones the repository name into a work directory with the remote
// origin, adds the further remotes (name to "owner/repo"), and chdirs into
// it.
func (s *server) clone(t *testing.T, name string, remotes ...[2]string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	runGit(t, "clone", "-q", s.url(name), work)
	for _, r := range remotes {
		runGit(t, "-C", work, "remote", "add", r[0], s.url(r[1]))
	}
	t.Chdir(work)
	return work
}

// localHead returns the SHA of the local branch.
func localHead(t *testing.T, branch string) string {
	t.Helper()
	return runGit(t, "rev-parse", "refs/heads/"+branch)
}

// noAPI fails the test on any HTTP request.
func noAPI(t *testing.T) {
	t.Helper()
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })
	httpmock.ReplaceDefaultTransport(t, reg)
}

func TestSyncLocal_FastForward(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	want := s.commit(t, "my-org/my-repo", "main", "upstream change")

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"), httpmock.StringResponse(http.StatusOK, `{"default_branch":"main"}`))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios)))

	assert.Equal(t, "Synced the \"main\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, want, localHead(t, "main"))
	assert.Equal(t, want, runGit(t, "rev-parse", "HEAD"))
}

func TestSyncLocal_UpToDate(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	before := localHead(t, "main")
	noAPI(t)

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "--branch", "main"))

	assert.Equal(t, "Synced the \"main\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, before, localHead(t, "main"))
}

func TestSyncLocal_Diverged(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	runGit(t, "commit", "-q", "--allow-empty", "-m", "local change")
	local := localHead(t, "main")
	s.commit(t, "my-org/my-repo", "main", "upstream change")
	noAPI(t)

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(testFactory(ios), "-b", "main")
	require.EqualError(t, err, "can't sync because there are diverging changes; use `--force` to overwrite the destination branch")
	assert.Empty(t, stdout.String())
	assert.Equal(t, local, localHead(t, "main"))
}

func TestSyncLocal_Force(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	runGit(t, "commit", "-q", "--allow-empty", "-m", "local change")
	want := s.commit(t, "my-org/my-repo", "main", "upstream change")
	noAPI(t)

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "-b", "main", "--force"))

	assert.Equal(t, "Synced the \"main\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, want, localHead(t, "main"))
	assert.Equal(t, want, runGit(t, "rev-parse", "HEAD"))
}

func TestSyncLocal_DirtyTree(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		diverge bool
		dirty   func(t *testing.T, work string)
	}{
		{
			name: "untracked file, fast-forward",
			args: []string{"-b", "main"},
			dirty: func(t *testing.T, work string) {
				require.NoError(t, os.WriteFile(filepath.Join(work, "new.txt"), []byte("x"), 0o644))
			},
		},
		{
			name:    "staged file, --force",
			args:    []string{"-b", "main", "--force"},
			diverge: true,
			dirty: func(t *testing.T, work string) {
				require.NoError(t, os.WriteFile(filepath.Join(work, "new.txt"), []byte("x"), 0o644))
				runGit(t, "add", "new.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t)
			s.create(t, "my-org/my-repo")
			work := s.clone(t, "my-org/my-repo")
			if tt.diverge {
				runGit(t, "commit", "-q", "--allow-empty", "-m", "local change")
			}
			before := localHead(t, "main")
			s.commit(t, "my-org/my-repo", "main", "upstream change")
			tt.dirty(t, work)
			noAPI(t)

			ios, _, stdout, _ := iostreams.Test()
			err := runCmd(testFactory(ios), tt.args...)
			require.EqualError(t, err, "refusing to sync due to uncommitted/untracked local changes\ntip: use `git stash --all` before retrying the sync and run `git stash pop` afterwards")
			assert.Empty(t, stdout.String())
			assert.Equal(t, before, localHead(t, "main"))
			_, err = os.Stat(filepath.Join(work, "new.txt"))
			assert.NoError(t, err)
		})
	}
}

func TestSyncLocal_BranchNotCheckedOut(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	runGit(t, "checkout", "-q", "-b", "work")
	require.NoError(t, os.WriteFile("wip.txt", []byte("x"), 0o644))
	want := s.commit(t, "my-org/my-repo", "main", "upstream change")
	noAPI(t)

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "-b", "main"))

	assert.Equal(t, "Synced the \"main\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, want, localHead(t, "main"))
	assert.Equal(t, "work", runGit(t, "symbolic-ref", "--short", "HEAD"))
}

func TestSyncLocal_BranchNotCheckedOutDiverged(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	runGit(t, "commit", "-q", "--allow-empty", "-m", "local change")
	local := localHead(t, "main")
	runGit(t, "checkout", "-q", "-b", "work")
	s.commit(t, "my-org/my-repo", "main", "upstream change")
	noAPI(t)

	ios, _, _, _ := iostreams.Test()
	err := runCmd(testFactory(ios), "-b", "main")
	require.EqualError(t, err, "can't sync because there are diverging changes; use `--force` to overwrite the destination branch")
	assert.Equal(t, local, localHead(t, "main"))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "-b", "main", "--force"))
	assert.Equal(t, "Synced the \"main\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Equal(t, s.head(t, "my-org/my-repo", "main"), localHead(t, "main"))
	assert.Equal(t, "work", runGit(t, "symbolic-ref", "--short", "HEAD"))
}

func TestSyncLocal_NewBranch(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.clone(t, "my-org/my-repo")
	want := s.commit(t, "my-org/my-repo", "dev", "new branch")
	noAPI(t)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "-b", "dev"))

	assert.Equal(t, "Synced the \"dev\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Equal(t, want, localHead(t, "dev"))
	assert.Equal(t, "origin/dev", runGit(t, "rev-parse", "--abbrev-ref", "dev@{upstream}"))
	assert.Equal(t, "main", runGit(t, "symbolic-ref", "--short", "HEAD"))
}

func TestSyncLocal_Source(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.create(t, "my-user/my-fork")
	s.clone(t, "my-user/my-fork", [2]string{"upstream", "my-org/my-repo"})
	want := s.commit(t, "my-org/my-repo", "main", "upstream change")
	noAPI(t)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "--source", "My-Org/my-repo", "-b", "main", "--force"))

	assert.Equal(t, "Synced the \"main\" branch from \"My-Org/my-repo\" to local repository\n", stdout.String())
	assert.Equal(t, want, localHead(t, "main"))
}

func TestSyncLocal_DefaultRemote(t *testing.T) {
	s := newServer(t)
	s.create(t, "my-org/my-repo")
	s.create(t, "my-user/my-fork")
	s.clone(t, "my-user/my-fork", [2]string{"upstream", "my-org/my-repo"})
	runGit(t, "config", "remote.upstream."+cmdutil.RemoteResolvedKey, "base")
	want := s.commit(t, "my-org/my-repo", "main", "upstream change")

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"), httpmock.StringResponse(http.StatusOK, `{"default_branch":"main"}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(testFactory(ios), "--force"))

	assert.Equal(t, "Synced the \"main\" branch from \"my-org/my-repo\" to local repository\n", stdout.String())
	assert.Equal(t, want, localHead(t, "main"))
}

func TestSyncLocal_Errors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "source without a remote",
			args:    []string{"--source", "other-org/other-repo", "-b", "main"},
			wantErr: "can't find corresponding remote for other-org/other-repo",
		},
		{
			name:    "source not owner/repo",
			args:    []string{"--source", "other-repo", "-b", "main"},
			wantErr: `expected owner/repo format, got "other-repo"`,
		},
		{
			name:    "branch missing on the remote",
			args:    []string{"-b", "no-such-branch"},
			wantErr: "git fetch failed: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t)
			s.create(t, "my-org/my-repo")
			s.clone(t, "my-org/my-repo")
			noAPI(t)

			ios, _, stdout, _ := iostreams.Test()
			err := runCmd(testFactory(ios), tt.args...)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), tt.wantErr), "error: %q", err.Error())
			assert.Empty(t, stdout.String())
		})
	}
}

func TestSyncLocal_NotInGitRepository(t *testing.T) {
	isolateGit(t)
	t.Chdir(t.TempDir())
	noAPI(t)

	ios, _, _, _ := iostreams.Test()
	err := runCmd(testFactory(ios), "-b", "main")
	require.ErrorContains(t, err, "could not determine repository")
}
