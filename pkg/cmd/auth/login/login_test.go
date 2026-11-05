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
	"github.com/zalando/go-keyring"
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

func TestLoginRun_SetsDefaultHost(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{
		DefaultHostName: "a.example.com",
		Hosts: map[string]*config.HostConfig{
			"a.example.com": {Token: "other", User: "jane"},
		},
	}))

	ios, _, _, _ := iostreams.Test()
	err := LoginRun(&LoginOptions{
		IO:         ios,
		Host:       "z.example.com",
		Token:      "tok",
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
	})
	require.NoError(t, err)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "z.example.com", cfg.DefaultHostName)
}

// failingStore is a keyring that refuses every call with err.
type failingStore struct{ err error }

func (s failingStore) Get(string, string) (string, error) { return "", s.err }
func (s failingStore) Set(string, string, string) error   { return s.err }
func (s failingStore) Delete(string, string) error        { return s.err }

func TestLoginRun_TokenStorage(t *testing.T) {
	tests := []struct {
		name        string
		keyring     func() config.TokenStore
		insecure    bool
		existing    *config.HostConfig
		wantFile    string
		wantKeyring string
		wantWarning string
	}{
		{
			name:        "keyring",
			keyring:     func() config.TokenStore { return config.NewKeyringStore() },
			wantKeyring: "secret-token",
		},
		{
			name:        "keyring moves a config file token",
			keyring:     func() config.TokenStore { return config.NewKeyringStore() },
			existing:    &config.HostConfig{Token: "old-token", User: "john", Extra: map[string]any{"git_protocol": "ssh"}},
			wantKeyring: "secret-token",
		},
		{
			name:        "keyring unavailable",
			keyring:     func() config.TokenStore { return failingStore{err: errors.New("OS keyring: no secret service")} },
			wantFile:    "secret-token",
			wantWarning: "warning: could not store the token in the OS keyring (OS keyring: no secret service); stored it in the config file",
		},
		{
			name:        "keyring timeout",
			keyring:     func() config.TokenStore { return failingStore{err: config.ErrKeyringTimeout} },
			wantFile:    "secret-token",
			wantWarning: "warning: could not store the token in the OS keyring (timed out waiting for the OS keyring); stored it in the config file",
		},
		{
			name:     "insecure storage",
			keyring:  func() config.TokenStore { return config.NewKeyringStore() },
			insecure: true,
			wantFile: "secret-token",
		},
		{
			name:     "no keyring",
			keyring:  func() config.TokenStore { return nil },
			wantFile: "secret-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/user"),
				httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

			configPath := filepath.Join(t.TempDir(), "config.yml")
			if tt.existing != nil {
				require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{"app.copia.io": tt.existing}}))
			}

			ios, _, stdout, stderr := iostreams.Test()
			err := LoginRun(&LoginOptions{
				IO:              ios,
				Host:            "app.copia.io",
				Token:           "secret-token",
				ConfigPath:      configPath,
				HTTPClient:      &http.Client{Transport: reg},
				Keyring:         tt.keyring(),
				InsecureStorage: tt.insecure,
			})
			require.NoError(t, err)
			assert.Equal(t, "Logged in as john on app.copia.io\n", stdout.String())

			cfg, err := config.Load(configPath)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFile, cfg.Hosts["app.copia.io"].Token)
			assert.Equal(t, "john", cfg.Hosts["app.copia.io"].User)
			if tt.existing != nil {
				assert.Equal(t, tt.existing.Extra, cfg.Hosts["app.copia.io"].Extra)
			}
			if tt.wantFile == "" {
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				assert.NotContains(t, string(data), "token")
			}

			got, err := keyring.Get("copia-cli:app.copia.io", "john")
			if tt.wantKeyring == "" {
				assert.ErrorIs(t, err, keyring.ErrNotFound)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantKeyring, got)
			}

			if tt.wantWarning == "" {
				assert.Empty(t, stderr.String())
			} else {
				assert.Contains(t, stderr.String(), tt.wantWarning)
				assert.Contains(t, stderr.String(), configPath)
				assert.NotContains(t, stderr.String(), "secret-token")
			}
		})
	}
}

func TestNewCmdLogin_InsecureStorage(t *testing.T) {
	keyring.MockInit()
	configPath := setTempConfigHome(t)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Keyring: config.NewKeyringStore()}
	require.NoError(t, runLoginCmd(t, f, "--token", "flag-token", "--insecure-storage"))

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "flag-token", cfg.Hosts["app.copia.io"].Token)
	_, err = keyring.Get("copia-cli:app.copia.io", "john")
	assert.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestNewCmdLogin_Keyring(t *testing.T) {
	keyring.MockInit()
	configPath := setTempConfigHome(t)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`))

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Keyring: config.NewKeyringStore()}
	require.NoError(t, runLoginCmd(t, f, "--token", "flag-token"))

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Empty(t, cfg.Hosts["app.copia.io"].Token)
	got, err := keyring.Get("copia-cli:app.copia.io", "john")
	require.NoError(t, err)
	assert.Equal(t, "flag-token", got)
}
