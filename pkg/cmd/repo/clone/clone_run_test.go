package clone

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
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

// setupRemote creates a bare repo at <parent>/my-org/my-repo.git with one
// commit and rewrites the Copia URLs CloneRun builds to point at it via
// git's environment config. It then chdirs into an empty work dir.
func setupRemote(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	parent := t.TempDir()
	src := t.TempDir()
	runGit(t, "init", "-q", src)
	runGit(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, "clone", "-q", "--bare", src, filepath.Join(parent, "my-org", "my-repo.git"))

	base := "url.file://" + filepath.ToSlash(parent) + "/.insteadOf"
	t.Setenv("GIT_CONFIG_COUNT", "3")
	t.Setenv("GIT_CONFIG_KEY_0", base)
	t.Setenv("GIT_CONFIG_VALUE_0", "https://token:test-token@app.copia.io/")
	t.Setenv("GIT_CONFIG_KEY_1", base)
	t.Setenv("GIT_CONFIG_VALUE_1", "https://app.copia.io/")
	t.Setenv("GIT_CONFIG_KEY_2", base)
	t.Setenv("GIT_CONFIG_VALUE_2", "git@app.copia.io:")

	work := t.TempDir()
	t.Chdir(work)
	return work
}

// storedOrigin returns the raw origin URL from the clone's config. Unlike
// `git remote get-url`, it is not rewritten by the insteadOf rules above.
func storedOrigin(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, "-C", dir, "config", "--get", "remote.origin.url")
}

func newCloneCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdClone(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdClone_Run(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantDir string
	}{
		{name: "explicit directory", args: []string{"my-org/my-repo", "target"}, wantDir: "target"},
		{name: "derived directory", args: []string{"my-org/my-repo"}, wantDir: "my-repo"},
		{name: "https URL", args: []string{"https://app.copia.io/my-org/my-repo.git"}, wantDir: "my-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := setupRemote(t)
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

			err := newCloneCmd(f, tt.args...)
			require.NoError(t, err)

			dir := filepath.Join(work, tt.wantDir)
			assert.DirExists(t, filepath.Join(dir, ".git"))
			assert.Equal(t, "init", runGit(t, "-C", dir, "log", "-1", "--format=%s"))

			origin := storedOrigin(t, dir)
			assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", origin)
			assert.NotContains(t, origin, "test-token")
		})
	}
}

func TestNewCmdClone_CloneFailure(t *testing.T) {
	work := setupRemote(t)
	ios, _, _, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	err := newCloneCmd(f, "my-org/missing")
	require.Error(t, err)
	assert.NotEmpty(t, stderr.String())
	assert.NoDirExists(t, filepath.Join(work, "missing"))
}

func TestNewCmdClone_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}

	err := newCloneCmd(f, "my-org/my-repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}

func TestNewCmdClone_Args(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	assert.Error(t, newCloneCmd(f))
	assert.Error(t, newCloneCmd(f, "a/b", "dir", "extra"))
}

func TestCloneRun_NoToken(t *testing.T) {
	work := setupRemote(t)
	ios, _, _, _ := iostreams.Test()

	err := CloneRun(&CloneOptions{IO: ios, Host: "app.copia.io", Repo: "my-org/my-repo"})
	require.NoError(t, err)
	assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", storedOrigin(t, filepath.Join(work, "my-repo")))
}

func TestCloneRun_SSHKeepsURL(t *testing.T) {
	work := setupRemote(t)
	ios, _, _, _ := iostreams.Test()

	err := CloneRun(&CloneOptions{
		IO:    ios,
		Host:  "app.copia.io",
		Token: "test-token",
		Repo:  "git@app.copia.io:my-org/my-repo.git",
		Dir:   "ssh-clone",
	})
	require.NoError(t, err)
	assert.Equal(t, "git@app.copia.io:my-org/my-repo.git", storedOrigin(t, filepath.Join(work, "ssh-clone")))
}

func TestBuildAuthCloneURL(t *testing.T) {
	assert.Equal(t, "https://token:tok@app.copia.io/my-org/my-repo.git",
		buildAuthCloneURL("app.copia.io", "tok", "my-org/my-repo"))
	assert.Equal(t, "https://token:tok@app.copia.io/my-org/my-repo.git",
		buildAuthCloneURL("app.copia.io", "tok", "https://app.copia.io/my-org/my-repo.git"))
}
