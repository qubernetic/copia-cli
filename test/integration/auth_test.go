//go:build integration

package integration

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/internal/copiacmd"
	"github.com/qubernetic/copia-cli/pkg/cmd/auth/login"
	"github.com/qubernetic/copia-cli/pkg/cmd/auth/status"
)

func TestAuth_LoginRun_ValidToken(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &login.LoginOptions{
		IO:         ios,
		Host:       env.Host,
		Token:      env.Token,
		ConfigPath: filepath.Join(t.TempDir(), "config.yml"),
		HTTPClient: &http.Client{},
	}

	err := login.LoginRun(opts)
	require.NoError(t, err)
	assertOutputContains(t, stdout.String(), "Logged in as")
}

func TestAuth_LoginRun_InvalidToken(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &login.LoginOptions{
		IO:         ios,
		Host:       env.Host,
		Token:      "invalid-token-does-not-exist",
		ConfigPath: filepath.Join(t.TempDir(), "config.yml"),
		HTTPClient: &http.Client{},
	}

	err := login.LoginRun(opts)
	require.Error(t, err)
	assert.ErrorContains(t, err, "authentication failed")
}

func TestAuth_StatusRun(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	// StatusRun reads from config file, so write one
	configPath := filepath.Join(t.TempDir(), "config.yml")
	configContent := []byte("hosts:\n  " + env.Host + ":\n    token: " + env.Token + "\n    user: testuser\n")
	require.NoError(t, os.WriteFile(configPath, configContent, 0600))

	opts := &status.StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{},
	}

	err := status.StatusRun(opts)
	require.NoError(t, err)
	assertOutputContains(t, stdout.String(), env.Host)
	assertOutputContains(t, stdout.String(), "Token valid")
}

func TestAuth_QueryParamToken_Rejected(t *testing.T) {
	env := loadTestEnv(t)

	u := url.URL{
		Scheme:   "https",
		Host:     env.Host,
		Path:     "/api/v1/user",
		RawQuery: url.Values{"access_token": {env.Token}}.Encode(),
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	resp, err := testHTTPClient().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Copia ignores the query token and treats the call as anonymous, which it
	// answers with 403 rather than 401.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assertStatusCode(t, resp.StatusCode, http.StatusForbidden)
	assertOutputContains(t, string(body), "Only signed in user is allowed to call APIs.")
}

// TestAuth_LoginFallsBackToConfigFile logs in through the OS keyring store
// where no keyring answers, as on the CI runner: the token falls back to the
// temporary config file with a warning, auth status reports it valid, and a
// command reads it from there for one API call.
func TestAuth_LoginFallsBackToConfigFile(t *testing.T) {
	env := loadTestEnv(t)
	if runtime.GOOS != "linux" {
		// The session bus below only disables the Secret Service; a macOS or
		// Windows keyring would answer and keep the token.
		t.Skip("needs Linux, where the keyring can be disabled for the test")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(home, "no-session-bus"))
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_OUTPUT", "")
	configPath := config.DefaultPath()

	ios, stdout, stderr := testIO()
	err := login.LoginRun(&login.LoginOptions{
		IO:         ios,
		Host:       env.Host,
		Token:      env.Token,
		ConfigPath: configPath,
		HTTPClient: testHTTPClient(),
		Keyring:    config.NewKeyringStore(),
	})
	require.NoError(t, err, "auth login failed")
	assertOutputContains(t, stdout.String(), "Logged in as")
	assertOutputContains(t, stderr.String(), "could not store the token in the OS keyring")
	assertOutputContains(t, stderr.String(), "stored it in the config file "+configPath)
	assert.False(t, strings.Contains(stderr.String(), env.Token), "the warning holds the token")

	cfg, err := config.Load(configPath)
	require.NoError(t, err, "reading the temporary config failed")
	stored, source, err := config.LookupToken(cfg, env.Host, nil)
	require.True(t, err == nil && stored == env.Token && source == config.SourceFile, "the token is not in the temporary config file")

	ios, stdout, _ = testIO()
	err = status.StatusRun(&status.StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: testHTTPClient(),
		Keyring:    config.NewKeyringStore(),
	})
	require.NoError(t, err, "auth status failed")
	assertOutputContains(t, stdout.String(), " (config file)\n  Token valid\n")

	ios, stdout, _ = testIO()
	require.NoError(t, copiacmd.Execute(copiacmd.NewFactory(ios), []string{"api", "/user"}), "api /user with the stored token failed")
	assertOutputNotEmpty(t, stdout.String())
}

// TestAuth_LoginMakesAccountActive logs in with the token stored in a
// temporary config file and checks that auth status lists the account as the
// host's active account with a valid token.
func TestAuth_LoginMakesAccountActive(t *testing.T) {
	env := loadTestEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	configPath := filepath.Join(home, "config.yml")

	ios, stdout, _ := testIO()
	err := login.LoginRun(&login.LoginOptions{
		IO:              ios,
		Host:            env.Host,
		Token:           env.Token,
		ConfigPath:      configPath,
		HTTPClient:      testHTTPClient(),
		InsecureStorage: true,
	})
	require.NoError(t, err, "auth login failed")
	assertOutputContains(t, stdout.String(), "Logged in as")

	cfg, err := config.Load(configPath)
	require.NoError(t, err, "reading the temporary config failed")
	hc := cfg.Hosts[env.Host]
	require.True(t, hc != nil && hc.User != "" && len(hc.Logins()) == 1, "auth login stored no active account")

	ios, stdout, _ = testIO()
	err = status.StatusRun(&status.StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: testHTTPClient(),
		Host:       env.Host,
	})
	require.NoError(t, err, "auth status failed")
	assertOutputContains(t, stdout.String(), "\n  Active account: true\n  Token: ")
	assertOutputContains(t, stdout.String(), " (config file)\n  Token valid\n")
	assert.False(t, strings.Contains(stdout.String(), env.Token), "auth status printed the token")
}
