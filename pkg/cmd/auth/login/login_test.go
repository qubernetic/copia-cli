package login

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginRun_NonInteractive_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	configPath := t.TempDir() + "/config.yml"

	opts := &LoginOptions{
		IO:         ios,
		Host:       "app.copia.io",
		Token:      "test-token-123",
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
	}

	err := LoginRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Logged in as john")

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "test-token-123", cfg.Hosts["app.copia.io"].Token)
	assert.Equal(t, "john", cfg.Hosts["app.copia.io"].User)
}

func TestLoginRun_InvalidToken(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusUnauthorized, `{"message":"Unauthorized"}`),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &LoginOptions{
		IO:         ios,
		Host:       "app.copia.io",
		Token:      "bad-token",
		ConfigPath: t.TempDir() + "/config.yml",
		HTTPClient: &http.Client{Transport: reg},
	}

	err := LoginRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errBody) Close() error             { return nil }

func setTempConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	return filepath.Join(dir, "copia", "config.yml")
}

func runLoginCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdLogin(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdLogin_Flags(t *testing.T) {
	configPath := setTempConfigHome(t)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios}, "--host", "copia.example.com", "--token", "flag-token")
	require.NoError(t, err)
	assert.Equal(t, "Logged in as john on copia.example.com\n", stdout.String())
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "copia.example.com", reg.Requests[0].URL.Host)
	assert.Equal(t, "token flag-token", reg.Requests[0].Header.Get("Authorization"))

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	require.Contains(t, cfg.Hosts, "copia.example.com")
	assert.Equal(t, "flag-token", cfg.Hosts["copia.example.com"].Token)
	assert.Equal(t, "john", cfg.Hosts["copia.example.com"].User)
}

func TestNewCmdLogin_FactoryOverridesAndDefaultHost(t *testing.T) {
	configPath := setTempConfigHome(t)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"jane","id":2}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios, Token: "factory-token"})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Logged in as jane on app.copia.io")
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "app.copia.io", reg.Requests[0].URL.Host)
	assert.Equal(t, "token factory-token", reg.Requests[0].Header.Get("Authorization"))

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "factory-token", cfg.Hosts["app.copia.io"].Token)
}

func TestNewCmdLogin_FactoryHost(t *testing.T) {
	setTempConfigHome(t)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	ios, _, _, _ := iostreams.Test()
	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios, Host: "factory.example.com"}, "--token", "tok")
	require.NoError(t, err)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "factory.example.com", reg.Requests[0].URL.Host)
}

func TestNewCmdLogin_InteractivePrompt(t *testing.T) {
	configPath := setTempConfigHome(t)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	ios, stdin, stdout, stderr := iostreams.Test()
	ios.SetStdinTTY(true)
	stdin.WriteString("  typed-token  \n")

	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios})
	require.NoError(t, err)
	assert.Equal(t, "Enter token for app.copia.io: ", stderr.String())
	assert.Contains(t, stdout.String(), "Logged in as john")
	assert.Equal(t, "token typed-token", reg.Requests[0].Header.Get("Authorization"))

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "typed-token", cfg.Hosts["app.copia.io"].Token)
}

func TestNewCmdLogin_InteractiveEmptyInput(t *testing.T) {
	setTempConfigHome(t)
	ios, _, _, stderr := iostreams.Test()
	ios.SetStdinTTY(true)

	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token required")
	assert.Contains(t, stderr.String(), "Enter token for app.copia.io")
}

func TestNewCmdLogin_InteractiveScanError(t *testing.T) {
	setTempConfigHome(t)
	ios, stdin, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	stdin.WriteString(strings.Repeat("x", bufio.MaxScanTokenSize+1))

	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios})
	require.Error(t, err)
	assert.ErrorIs(t, err, bufio.ErrTooLong)
}

func TestNewCmdLogin_NonTTYWithoutToken(t *testing.T) {
	configPath := setTempConfigHome(t)
	ios, stdin, _, stderr := iostreams.Test()
	stdin.WriteString("ignored-token\n")

	err := runLoginCmd(t, &cmdutil.Factory{IOStreams: ios})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token required")
	assert.Empty(t, stderr.String())
	assert.NoFileExists(t, configPath)
}

func TestLoginRun_Errors(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		responder  httpmock.Responder
		configPath func(t *testing.T) string
		wantErr    string
	}{
		{
			name:    "invalid host",
			host:    "bad host",
			wantErr: "invalid character",
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io",
		},
		{
			name:      "server error",
			responder: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr:   "authentication failed for app.copia.io (HTTP 500)",
		},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: errBody{}}, nil
			},
			wantErr: "read failed",
		},
		{
			name:      "malformed json",
			responder: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr:   "parsing user response",
		},
		{
			name:      "config load error",
			responder: httpmock.StringResponse(http.StatusOK, `{"login":"john"}`),
			configPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "config.yml")
				require.NoError(t, os.WriteFile(path, []byte("hosts: [\n"), 0600))
				return path
			},
			wantErr: "parsing config",
		},
		{
			name:      "config save error",
			responder: httpmock.StringResponse(http.StatusOK, `{"login":"john"}`),
			configPath: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yml")
				require.NoError(t, os.Symlink(filepath.Join(dir, "missing", "config.yml"), path))
				return path
			},
			wantErr: "writing config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/user"), tt.responder)
			}

			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			configPath := filepath.Join(t.TempDir(), "config.yml")
			if tt.configPath != nil {
				configPath = tt.configPath(t)
			}

			ios, _, stdout, _ := iostreams.Test()
			err := LoginRun(&LoginOptions{
				IO:         ios,
				Host:       host,
				Token:      "tok",
				ConfigPath: configPath,
				HTTPClient: &http.Client{Transport: reg},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestLoginRun_KeepsOtherHosts(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"other.example.com": {Token: "other", User: "jane"},
	}}))

	ios, _, _, _ := iostreams.Test()
	err := LoginRun(&LoginOptions{
		IO:         ios,
		Host:       "app.copia.io",
		Token:      "tok",
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
	})
	require.NoError(t, err)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Len(t, cfg.Hosts, 2)
	assert.Equal(t, "jane", cfg.Hosts["other.example.com"].User)
}
