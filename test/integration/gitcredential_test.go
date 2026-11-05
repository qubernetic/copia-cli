//go:build integration

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qubernetic/copia-cli/pkg/cmd/auth/login"
)

// TestAuth_GitCredential logs in with the token stored in a temporary config
// file and runs git ls-remote of the test repository with the test binary as
// the only credential helper of the host. Copia has no anonymous access, so
// the same ls-remote without the helper fails first. The output of git is
// kept in buffers and never printed, as it names the test repository's refs.
func TestAuth_GitCredential(t *testing.T) {
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

	exe, err := os.Executable()
	require.NoError(t, err, "finding the test binary failed")
	repoURL := "https://" + env.Host + "/" + env.Owner + "/" + env.Repo + ".git"

	lsRemote := func(helper string) (string, bool) {
		cmd := exec.Command("git", "-c", "credential.helper=", "-c", "credential.https://"+env.Host+".helper="+helper, "ls-remote", "--", repoURL)
		cmd.Env = append(os.Environ(), runCLIEnv+"=1")
		var out, errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		err := cmd.Run()
		return out.String(), err == nil
	}

	_, ok := lsRemote("")
	require.False(t, ok, "git ls-remote without a credential helper succeeded, so the test proves nothing")

	out, ok := lsRemote("!'" + exe + "' auth git-credential")
	require.True(t, ok, "git ls-remote with auth git-credential as the credential helper failed")
	assertOutputContains(t, out, "\tHEAD\n")
}
