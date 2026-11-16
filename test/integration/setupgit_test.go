//go:build integration

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qubernetic/copia-cli/internal/copiacmd"
	"github.com/qubernetic/copia-cli/pkg/cmd/auth/login"
)

// TestAuth_SetupGit logs in with the token stored in a temporary config file,
// runs auth setup-git into a temporary global git config and then a plain
// git ls-remote of the test repository, with no helper on the command line.
// The test binary is the copia-cli setup-git writes into the git config. The
// same ls-remote before setup-git fails first, as Copia has no anonymous
// access. The output of git is kept in buffers and never printed.
func TestAuth_SetupGit(t *testing.T) {
	env := loadTestEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(home, "no-session-bus"))
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")

	ios, stdout, _ := testIO()
	err := login.LoginRun(&login.LoginOptions{
		IO:              ios,
		Host:            env.Host,
		Token:           env.Token,
		ConfigPath:      filepath.Join(home, ".config", "copia", "config.yml"),
		HTTPClient:      testHTTPClient(),
		InsecureStorage: true,
	})
	require.NoError(t, err, "auth login failed")
	assertOutputContains(t, stdout.String(), "Logged in as")

	repoURL := "https://" + env.Host + "/" + env.Owner + "/" + env.Repo + ".git"
	lsRemote := func() (string, bool) {
		cmd := exec.Command("git", "ls-remote", "--", repoURL)
		cmd.Env = append(os.Environ(), runCLIEnv+"=1")
		var out, errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		err := cmd.Run()
		return out.String(), err == nil
	}

	_, ok := lsRemote()
	require.False(t, ok, "git ls-remote before auth setup-git succeeded, so the test proves nothing")

	ios, stdout, _ = testIO()
	require.NoError(t, copiacmd.Execute(copiacmd.NewFactory(ios), []string{"auth", "setup-git"}), "auth setup-git failed")
	assertOutputEmpty(t, stdout.String())

	out, ok := lsRemote()
	require.True(t, ok, "git ls-remote after auth setup-git failed")
	assertOutputContains(t, out, "\tHEAD\n")
}
