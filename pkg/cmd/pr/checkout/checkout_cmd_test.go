package checkout

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateGit makes git independent of the host's global and system config.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	globalCfg := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(globalCfg, nil, 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", globalCfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "author@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "committer@example.com")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// setupRepos creates a bare "remote" holding refs/pull/7/head and a work
// clone of it on main, then chdirs into the work clone.
func setupRepos(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	work := filepath.Join(root, "work")

	git(t, root, "init", "--bare", bare)
	git(t, root, "init", seed)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "a.txt"), []byte("base\n"), 0o600))
	git(t, seed, "add", "a.txt")
	git(t, seed, "commit", "-m", "base")
	git(t, seed, "push", bare, "HEAD:refs/heads/main")

	git(t, seed, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(seed, "a.txt"), []byte("pr change\n"), 0o600))
	git(t, seed, "commit", "-am", "pr change")
	git(t, seed, "push", bare, "HEAD:refs/pull/7/head")

	git(t, root, "clone", bare, work)
	t.Chdir(work)
	return work
}

func runCheckout(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	ios, _, stdout, stderr := iostreams.Test()
	cmd := NewCmdCheckout(&cmdutil.Factory{IOStreams: ios})
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestNewCmdCheckout_Success(t *testing.T) {
	work := setupRepos(t)

	stdout, _, err := runCheckout(t, "7")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Checked out PR #7 as pr-7")
	assert.Equal(t, "pr-7", git(t, work, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, "pr change", git(t, work, "log", "-1", "--format=%s"))

	content, err := os.ReadFile(filepath.Join(work, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "pr change\n", string(content))
}

func TestNewCmdCheckout_FetchFailsForUnknownPR(t *testing.T) {
	work := setupRepos(t)

	stdout, stderr, err := runCheckout(t, "99")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching PR #99")
	assert.NotContains(t, stdout, "Checked out")
	assert.Contains(t, stderr, "pull/99/head")
	assert.Equal(t, "main", git(t, work, "rev-parse", "--abbrev-ref", "HEAD"))
}

func TestNewCmdCheckout_CheckoutFailsOnDirtyTree(t *testing.T) {
	work := setupRepos(t)
	require.NoError(t, os.WriteFile(filepath.Join(work, "a.txt"), []byte("local edit\n"), 0o600))

	stdout, _, err := runCheckout(t, "7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking out pr-7")
	assert.NotContains(t, stdout, "Checked out")
	assert.Equal(t, "main", git(t, work, "rev-parse", "--abbrev-ref", "HEAD"))
}

func TestNewCmdCheckout_InvalidNumber(t *testing.T) {
	_, _, err := runCheckout(t, "abc")
	require.Error(t, err)
	assert.Equal(t, "invalid PR number: abc", err.Error())
}

func TestNewCmdCheckout_RequiresOneArg(t *testing.T) {
	_, _, err := runCheckout(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
