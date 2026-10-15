//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Contains(t, stdout.String(), "Logged in as")
	t.Logf("Output: %s", stdout.String())
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
	assert.Contains(t, err.Error(), "authentication failed")
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
	assert.Contains(t, stdout.String(), env.Host)
	assert.Contains(t, stdout.String(), "Token valid")
	t.Logf("Output: %s", stdout.String())
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

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
