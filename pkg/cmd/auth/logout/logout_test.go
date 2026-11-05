package logout

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	orglist "github.com/qubernetic/copia-cli/pkg/cmd/org/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestLogoutRun_RemovesHost(t *testing.T) {
	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{
		Hosts: map[string]*config.HostConfig{
			"app.copia.io": {Token: "abc", User: "john"},
		},
	}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, stdout, _ := iostreams.Test()

	opts := &LogoutOptions{
		IO:         ios,
		Host:       "app.copia.io",
		ConfigPath: configPath,
	}

	err := logoutRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Logged out of app.copia.io")

	loaded, _ := config.Load(configPath)
	assert.Empty(t, loaded.Hosts)
}

func TestLogoutRun_HostNotFound(t *testing.T) {
	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{Hosts: map[string]*config.HostConfig{}}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, _, _ := iostreams.Test()

	opts := &LogoutOptions{
		IO:         ios,
		Host:       "unknown.host.com",
		ConfigPath: configPath,
	}

	err := logoutRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in to unknown.host.com")
}

func TestLogoutRun_MovesDefaultHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{
		DefaultHostName: "a.example.com",
		Hosts: map[string]*config.HostConfig{
			"c.example.com": {Token: "c", User: "u3"},
			"a.example.com": {Token: "a", User: "u1"},
			"b.example.com": {Token: "b", User: "u2"},
		},
	}))

	ios, _, _, _ := iostreams.Test()
	require.NoError(t, logoutRun(&LogoutOptions{IO: ios, Host: "a.example.com", ConfigPath: configPath}))

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "b.example.com", cfg.DefaultHostName)
	assert.NotContains(t, cfg.Hosts, "a.example.com")
}

// TestNewCmdLogout_EnvTokenNotSentToNewDefaultHost pins that logging out of
// the default host while COPIA_TOKEN is set without COPIA_HOST does not send
// COPIA_TOKEN to the host that becomes the default: the next command uses
// that host's stored token.
func TestNewCmdLogout_EnvTokenNotSentToNewDefaultHost(t *testing.T) {
	const envToken = "env-secret-token"
	keyring.MockInit()
	configPath := setTempConfigHome(t)
	t.Setenv("COPIA_TOKEN", envToken)
	require.NoError(t, config.Save(configPath, &config.Config{
		DefaultHostName: "a.example.com",
		Hosts: map[string]*config.HostConfig{
			"a.example.com": {Token: "token-a", User: "u1"},
			"b.example.com": {User: "u2"},
		},
	}))
	require.NoError(t, keyring.Set("copia-cli:b.example.com", "u2", "token-b"))

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user/orgs"), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Keyring:   config.NewKeyringStore(),
		Config:    func() (*config.Config, error) { return config.Load(configPath) },
	}
	require.NoError(t, runLogoutCmd(t, f))

	cmd := orglist.NewCmdList(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())

	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "b.example.com", reg.Requests[0].URL.Host)
	assert.Equal(t, "token token-b", reg.Requests[0].Header.Get("Authorization"))
	assert.Contains(t, stdout.String(), "Logged out of a.example.com")
	assert.Contains(t, stderr.String(), "warning: COPIA_TOKEN is ignored because no host is named")
	assert.NotContains(t, stdout.String(), envToken)
	assert.NotContains(t, stderr.String(), envToken)
}

func setTempConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("COPIA_HOST", "")
	return filepath.Join(dir, "copia", "config.yml")
}

func runLogoutCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdLogout(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdLogout(t *testing.T) {
	tests := []struct {
		name      string
		hosts     map[string]*config.HostConfig
		rawConfig   string
		defaultHost string
		factHost    string
		args      []string
		wantOut   string
		wantErr   string
		wantLeft  []string
	}{
		{
			name: "host flag",
			hosts: map[string]*config.HostConfig{
				"app.copia.io":      {Token: "a", User: "john"},
				"other.example.com": {Token: "b", User: "jane"},
			},
			args:     []string{"--host", "other.example.com"},
			wantOut:  "Logged out of other.example.com account jane\n",
			wantLeft: []string{"app.copia.io"},
		},
		{
			name: "factory host",
			hosts: map[string]*config.HostConfig{
				"app.copia.io":      {Token: "a", User: "john"},
				"other.example.com": {Token: "b", User: "jane"},
			},
			factHost: "app.copia.io",
			wantOut:  "Logged out of app.copia.io account john\n",
			wantLeft: []string{"other.example.com"},
		},
		{
			name: "default host from config",
			hosts: map[string]*config.HostConfig{
				"app.copia.io": {Token: "a", User: "john"},
			},
			wantOut:  "Logged out of app.copia.io account john\n",
			wantLeft: []string{},
		},
		{
			name:        "stored default host",
			defaultHost: "c.example.com",
			hosts: map[string]*config.HostConfig{
				"a.example.com": {Token: "a", User: "u1"},
				"b.example.com": {Token: "b", User: "u2"},
				"c.example.com": {Token: "c", User: "u3"},
			},
			wantOut:  "Logged out of c.example.com account u3\n",
			wantLeft: []string{"a.example.com", "b.example.com"},
		},
		{
			name: "unset default host",
			hosts: map[string]*config.HostConfig{
				"c.example.com": {Token: "c", User: "u3"},
				"b.example.com": {Token: "b", User: "u2"},
				"a.example.com": {Token: "a", User: "u1"},
			},
			wantOut:  "Logged out of a.example.com account u1\n",
			wantLeft: []string{"b.example.com", "c.example.com"},
		},
		{
			name:    "no config file",
			wantErr: "no host specified and no default configured",
		},
		{
			name:      "malformed config",
			rawConfig: "hosts: [\n",
			wantErr:   "parsing config",
		},
		{
			name: "unknown host flag",
			hosts: map[string]*config.HostConfig{
				"app.copia.io": {Token: "a", User: "john"},
			},
			args:     []string{"--host", "unknown.example.com"},
			wantErr:  "not logged in to unknown.example.com",
			wantLeft: []string{"app.copia.io"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := setTempConfigHome(t)
			if tt.hosts != nil {
				require.NoError(t, config.Save(configPath, &config.Config{DefaultHostName: tt.defaultHost, Hosts: tt.hosts}))
			}
			if tt.rawConfig != "" {
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
				require.NoError(t, os.WriteFile(configPath, []byte(tt.rawConfig), 0600))
			}

			ios, _, stdout, _ := iostreams.Test()
			err := runLogoutCmd(t, &cmdutil.Factory{IOStreams: ios, Host: tt.factHost}, tt.args...)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, stdout.String())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOut, stdout.String())
			}

			if tt.wantLeft != nil {
				cfg, err := config.Load(configPath)
				require.NoError(t, err)
				var left []string
				for h := range cfg.Hosts {
					left = append(left, h)
				}
				assert.ElementsMatch(t, tt.wantLeft, left)
			}
		})
	}
}

func TestLogoutRun_LoadError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("hosts: [\n"), 0600))

	ios, _, _, _ := iostreams.Test()
	err := logoutRun(&LogoutOptions{IO: ios, Host: "app.copia.io", ConfigPath: configPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestLogoutRun_SaveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": {Token: "a", User: "john"},
	}}))
	require.NoError(t, os.Chmod(configPath, 0400))

	ios, _, stdout, _ := iostreams.Test()
	err := logoutRun(&LogoutOptions{IO: ios, Host: "app.copia.io", ConfigPath: configPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing config")
	assert.Empty(t, stdout.String())
}

// failingStore is a keyring that refuses every call with err.
type failingStore struct{ err error }

func (s failingStore) Get(string, string) (string, error) { return "", s.err }
func (s failingStore) Set(string, string, string) error   { return s.err }
func (s failingStore) Delete(string, string) error        { return s.err }

func TestLogoutRun_TokenStorage(t *testing.T) {
	tests := []struct {
		name        string
		fileToken   string
		inKeyring   bool
		keyring     func() config.TokenStore
		wantWarning string
	}{
		{
			name:      "keyring only",
			inKeyring: true,
			keyring:   func() config.TokenStore { return config.NewKeyringStore() },
		},
		{
			name:      "config file and keyring",
			fileToken: "file-token",
			inKeyring: true,
			keyring:   func() config.TokenStore { return config.NewKeyringStore() },
		},
		{
			name:      "config file only",
			fileToken: "file-token",
			keyring:   func() config.TokenStore { return config.NewKeyringStore() },
		},
		{
			name:        "keyring unavailable",
			fileToken:   "file-token",
			keyring:     func() config.TokenStore { return failingStore{err: config.ErrKeyringTimeout} },
			wantWarning: "warning: could not delete the token of john on app.copia.io from the OS keyring: timed out waiting for the OS keyring\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			if tt.inKeyring {
				require.NoError(t, keyring.Set("copia-cli:app.copia.io", "john", "keyring-token"))
			}
			configPath := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
				"app.copia.io": {Token: tt.fileToken, User: "john"},
			}}))

			ios, _, stdout, stderr := iostreams.Test()
			err := logoutRun(&LogoutOptions{IO: ios, Host: "app.copia.io", ConfigPath: configPath, Keyring: tt.keyring()})
			require.NoError(t, err)
			assert.Equal(t, "Logged out of app.copia.io account john\n", stdout.String())
			assert.Equal(t, tt.wantWarning, stderr.String())

			loaded, err := config.Load(configPath)
			require.NoError(t, err)
			assert.Empty(t, loaded.Hosts)
			_, err = keyring.Get("copia-cli:app.copia.io", "john")
			assert.ErrorIs(t, err, keyring.ErrNotFound)
		})
	}
}

func TestNewCmdLogout_Keyring(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("COPIA_HOST", "")
	configPath := filepath.Join(dir, "copia", "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{"app.copia.io": {User: "john"}}}))
	require.NoError(t, keyring.Set("copia-cli:app.copia.io", "john", "keyring-token"))

	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdLogout(&cmdutil.Factory{IOStreams: ios, Keyring: config.NewKeyringStore()})
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())

	_, err := keyring.Get("copia-cli:app.copia.io", "john")
	assert.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestLogoutRun_OneOfSeveralAccounts(t *testing.T) {
	tests := []struct {
		name       string
		user       string
		wantOut    string
		wantErr    string
		wantActive string
		wantLogins []string
		wantGone   string
	}{
		{
			name:       "active account without --user",
			wantOut:    "Logged out of app.copia.io account joe\nSwitched active account for app.copia.io to jane\n",
			wantActive: "jane",
			wantLogins: []string{"jane", "john"},
			wantGone:   "joe",
		},
		{
			name:       "active account with --user",
			user:       "joe",
			wantOut:    "Logged out of app.copia.io account joe\nSwitched active account for app.copia.io to jane\n",
			wantActive: "jane",
			wantLogins: []string{"jane", "john"},
			wantGone:   "joe",
		},
		{
			name:       "other account",
			user:       "john",
			wantOut:    "Logged out of app.copia.io account john\n",
			wantActive: "joe",
			wantLogins: []string{"jane", "joe"},
			wantGone:   "john",
		},
		{
			name:       "unknown account",
			user:       "jim",
			wantErr:    "not logged in to app.copia.io account jim",
			wantActive: "joe",
			wantLogins: []string{"jane", "joe", "john"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			for _, login := range []string{"john", "joe", "jane"} {
				require.NoError(t, keyring.Set("copia-cli:app.copia.io", login, "keyring-token-"+login))
			}
			configPath := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
				"app.copia.io": {User: "joe", Users: map[string]*config.UserConfig{"john": {}, "joe": {}, "jane": {Token: "file-token"}}},
			}}))

			ios, _, stdout, stderr := iostreams.Test()
			err := logoutRun(&LogoutOptions{IO: ios, Host: "app.copia.io", User: tt.user, ConfigPath: configPath, Keyring: config.NewKeyringStore()})
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, stderr.String())

			loaded, err := config.Load(configPath)
			require.NoError(t, err)
			hc := loaded.Hosts["app.copia.io"]
			assert.Equal(t, tt.wantActive, hc.User)
			assert.Equal(t, tt.wantLogins, hc.Logins())
			assert.Equal(t, "file-token", hc.Users["jane"].Token)

			// Only the removed account's keyring entry is gone.
			for _, login := range []string{"john", "joe", "jane"} {
				_, err := keyring.Get("copia-cli:app.copia.io", login)
				if login == tt.wantGone {
					assert.ErrorIs(t, err, keyring.ErrNotFound)
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestNewCmdLogout_UserFlag(t *testing.T) {
	keyring.MockInit()
	configPath := setTempConfigHome(t)
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": {User: "joe", Users: map[string]*config.UserConfig{"john": {Token: "t1"}, "joe": {Token: "t2"}}},
	}}))

	ios, _, _, _ := iostreams.Test()
	require.NoError(t, runLogoutCmd(t, &cmdutil.Factory{IOStreams: ios}, "--user", "john"))

	loaded, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, []string{"joe"}, loaded.Hosts["app.copia.io"].Logins())
}

func TestLogoutRun_HostWithoutAccount(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": {Token: "legacy-token"},
	}}))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, logoutRun(&LogoutOptions{IO: ios, Host: "app.copia.io", ConfigPath: configPath}))
	assert.Equal(t, "Logged out of app.copia.io\n", stdout.String())

	loaded, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Empty(t, loaded.Hosts)
}
