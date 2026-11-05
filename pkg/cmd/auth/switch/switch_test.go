package authswitch

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Config files of the tests. The comment line is dropped by every save, so a
// file that still has it was not written.
const (
	twoHosts = `# untouched
default_host: a.example.com
hosts:
  a.example.com:
    user: joe
    users:
      joe:
        token: file-token-joe
      jane: {}
  b.example.com:
    user: u2
    users:
      u2: {}
`
	twoAccountsOneHost = `# untouched
default_host: a.example.com
hosts:
  a.example.com:
    user: joe
    users:
      joe: {}
      jane: {}
`
	oneAccountPerHost = `# untouched
default_host: a.example.com
hosts:
  a.example.com:
    user: u1
    users:
      u1: {}
  b.example.com:
    user: u2
    users:
      u2: {}
`
	threeAccountsOneHost = `# untouched
hosts:
  a.example.com:
    user: joe
    users:
      joe: {}
      jane: {}
      john: {}
`
	twoAccountsOtherHost = `# untouched
default_host: b.example.com
hosts:
  a.example.com:
    user: jane
    users:
      joe: {}
      jane: {}
  b.example.com:
    user: u2
    users:
      u2: {}
`
	sameLoginTwoHosts = `# untouched
default_host: c.example.com
hosts:
  a.example.com:
    user: joe
    users:
      joe: {}
  b.example.com:
    user: joe
    users:
      joe: {}
  c.example.com:
    user: u3
    users:
      u3: {}
`
	loginlessToken = `# untouched
default_host: b.example.com
hosts:
  a.example.com:
    token: file-token-old
    users:
      jane: {}
  b.example.com:
    user: u2
    users:
      u2: {}
`
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	path := filepath.Join(dir, "copia", "config.yml")
	if content != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	}
	return path
}

func runSwitchCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdSwitch(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdSwitch(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		factHost    string
		args        []string
		wantOut     string
		wantErr     string
		wantHost    string // the host whose active account is checked
		wantActive  string
		wantDefault string
	}{
		{
			name:        "host and user",
			config:      twoHosts,
			args:        []string{"--host", "a.example.com", "--user", "jane"},
			wantOut:     "Switched active account for a.example.com to jane\n",
			wantHost:    "a.example.com",
			wantActive:  "jane",
			wantDefault: "a.example.com",
		},
		{
			name:        "host and user of another host",
			config:      twoHosts,
			args:        []string{"--host", "b.example.com", "-u", "u2"},
			wantOut:     "Switched active account for b.example.com to u2\n",
			wantHost:    "b.example.com",
			wantActive:  "u2",
			wantDefault: "b.example.com",
		},
		{
			name:        "host alone with one account",
			config:      twoHosts,
			args:        []string{"--host", "b.example.com"},
			wantOut:     "Switched active account for b.example.com to u2\n",
			wantHost:    "b.example.com",
			wantActive:  "u2",
			wantDefault: "b.example.com",
		},
		{
			name:        "host alone with two accounts",
			config:      twoHosts,
			args:        []string{"--host", "a.example.com"},
			wantOut:     "Switched active account for a.example.com to jane\n",
			wantHost:    "a.example.com",
			wantActive:  "jane",
			wantDefault: "a.example.com",
		},
		{
			name:        "host alone with two accounts on a host that is not the default",
			config:      twoAccountsOtherHost,
			args:        []string{"--host", "a.example.com"},
			wantOut:     "Switched active account for a.example.com to joe\n",
			wantHost:    "a.example.com",
			wantActive:  "joe",
			wantDefault: "a.example.com",
		},
		{
			name:    "host alone with three accounts",
			config:  threeAccountsOneHost,
			args:    []string{"--host", "a.example.com"},
			wantErr: "unable to determine which account to switch to, specify --host and --user; accounts:\na.example.com jane\na.example.com joe\na.example.com john",
		},
		{
			name:        "factory host",
			config:      twoHosts,
			factHost:    "b.example.com",
			wantOut:     "Switched active account for b.example.com to u2\n",
			wantHost:    "b.example.com",
			wantActive:  "u2",
			wantDefault: "b.example.com",
		},
		{
			name:        "no flags with two accounts on one host",
			config:      twoAccountsOneHost,
			wantOut:     "Switched active account for a.example.com to jane\n",
			wantHost:    "a.example.com",
			wantActive:  "jane",
			wantDefault: "a.example.com",
		},
		{
			name:        "no flags with one account on each of two hosts",
			config:      oneAccountPerHost,
			wantOut:     "Switched active account for b.example.com to u2\n",
			wantHost:    "b.example.com",
			wantActive:  "u2",
			wantDefault: "b.example.com",
		},
		{
			name:    "no flags with more accounts",
			config:  twoHosts,
			wantErr: "unable to determine which account to switch to, specify --host and --user; accounts:\na.example.com jane\na.example.com joe\nb.example.com u2",
		},
		{
			name:        "user alone",
			config:      twoHosts,
			args:        []string{"--user", "u2"},
			wantOut:     "Switched active account for b.example.com to u2\n",
			wantHost:    "b.example.com",
			wantActive:  "u2",
			wantDefault: "b.example.com",
		},
		{
			name:    "user alone on two hosts, neither the default",
			config:  sameLoginTwoHosts,
			args:    []string{"--user", "joe"},
			wantErr: "unable to determine which account to switch to, specify --host and --user; accounts:\na.example.com joe\nb.example.com joe",
		},
		{
			name:    "unknown host",
			config:  twoHosts,
			args:    []string{"--host", "c.example.com", "--user", "joe"},
			wantErr: "not logged in to c.example.com",
		},
		{
			name:    "unknown host alone",
			config:  twoHosts,
			args:    []string{"--host", "c.example.com"},
			wantErr: "not logged in to c.example.com",
		},
		{
			name:    "unknown login on a host",
			config:  twoHosts,
			args:    []string{"--host", "a.example.com", "--user", "jim"},
			wantErr: "not logged in to a.example.com account jim",
		},
		{
			name:    "unknown login on any host",
			config:  twoHosts,
			args:    []string{"--user", "jim"},
			wantErr: "not logged in to any host as jim",
		},
		{
			name:    "old-shape token without a login",
			config:  loginlessToken,
			args:    []string{"--host", "a.example.com", "--user", "jane"},
			wantErr: "the token of a.example.com has no account; run auth login",
		},
		{
			name:    "no accounts",
			config:  "# untouched\nhosts: {}\n",
			wantErr: "not logged in to any hosts",
		},
		{
			name:    "no config file",
			wantErr: "not logged in to any hosts",
		},
		{
			name:    "malformed config",
			config:  "hosts: [\n",
			wantErr: "parsing config: yaml: line 1: did not find expected node content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			ios, _, stdout, stderr := iostreams.Test()
			err := runSwitchCmd(&cmdutil.Factory{IOStreams: ios, Host: tt.factHost}, tt.args...)
			assert.Empty(t, stderr.String())

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, stdout.String())
				// Nothing is written on failure.
				data, readErr := os.ReadFile(path)
				if tt.config == "" {
					assert.ErrorIs(t, readErr, os.ErrNotExist)
				} else {
					require.NoError(t, readErr)
					assert.Equal(t, tt.config, string(data))
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			cfg, err := config.Load(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantActive, cfg.Hosts[tt.wantHost].User)
			assert.Equal(t, tt.wantDefault, cfg.DefaultHostName)
		})
	}
}

func TestSwitchRun_KeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(`future_key: kept
hosts:
  a.example.com:
    user: joe
    future_host_key: kept
    users:
      joe:
        token: file-token-joe
      jane:
        future_user_key: kept
`), 0600))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, switchRun(&SwitchOptions{IO: ios, ConfigPath: path}))
	assert.Equal(t, "Switched active account for a.example.com to jane\n", stdout.String())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	cfg, err := config.Load(path)
	require.NoError(t, err)
	hc := cfg.Hosts["a.example.com"]
	assert.Equal(t, "jane", hc.User)
	assert.Equal(t, "kept", cfg.Extra["future_key"])
	assert.Equal(t, "kept", hc.Extra["future_host_key"])
	assert.Equal(t, "kept", hc.Users["jane"].Extra["future_user_key"])
	// The token stays with its account.
	assert.Equal(t, "file-token-joe", hc.Users["joe"].Token)
	assert.Empty(t, hc.Users["jane"].Token)
}

func TestSwitchRun_FileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	tests := []struct {
		name    string
		mode    os.FileMode
		wantErr string
	}{
		{name: "read-only file", mode: 0400, wantErr: "writing config"},
		{name: "unreadable file", mode: 0000, wantErr: "reading config:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, os.WriteFile(path, []byte(twoAccountsOneHost), tt.mode))

			ios, _, stdout, stderr := iostreams.Test()
			err := switchRun(&SwitchOptions{IO: ios, ConfigPath: path})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())

			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, tt.mode, info.Mode().Perm())
			require.NoError(t, os.Chmod(path, 0600))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, twoAccountsOneHost, string(data))
		})
	}
}

// envTokenWarning is what auth switch prints while COPIA_TOKEN is set.
const envTokenWarning = "warning: COPIA_TOKEN applies only with COPIA_HOST (or --host); without it commands use the token of the active account\n"

func TestSwitchRun_TokenFromEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(twoAccountsOneHost), 0600))
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "env-token")

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, switchRun(&SwitchOptions{IO: ios, ConfigPath: path}))
	assert.Equal(t, "Switched active account for a.example.com to jane\n", stdout.String())
	assert.Equal(t, envTokenWarning, stderr.String())
	assert.NotContains(t, stderr.String(), "env-token")
}

func TestSwitchRun_TokenFromEnvironmentOtherHost(t *testing.T) {
	// COPIA_TOKEN applies only to the host COPIA_HOST names, so moving
	// default_host to another host does not send it there.
	tests := []struct {
		name   string
		config string
		host   string
	}{
		{name: "no flags", config: oneAccountPerHost},
		{name: "host flag", config: twoHosts, host: "b.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, os.WriteFile(path, []byte(tt.config), 0600))
			t.Setenv("COPIA_HOST", "")
			t.Setenv("COPIA_TOKEN", "env-token")

			ios, _, stdout, stderr := iostreams.Test()
			require.NoError(t, switchRun(&SwitchOptions{IO: ios, Host: tt.host, ConfigPath: path}))
			assert.Contains(t, stdout.String(), "Switched active account for b.example.com to ")
			assert.Equal(t, envTokenWarning, stderr.String())
			assert.NotContains(t, stdout.String(), "env-token")
			cfg, err := config.Load(path)
			require.NoError(t, err)
			defaultHost, _ := cfg.DefaultHost()
			assert.Equal(t, "b.example.com", defaultHost)
		})
	}
}

func TestSwitchRun_TokenAndHostFromEnvironment(t *testing.T) {
	// With COPIA_HOST set, COPIA_TOKEN goes there whatever default_host is.
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(oneAccountPerHost), 0600))
	t.Setenv("COPIA_HOST", "a.example.com")
	t.Setenv("COPIA_TOKEN", "env-token")

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, switchRun(&SwitchOptions{IO: ios, ConfigPath: path}))
	assert.Equal(t, "Switched active account for b.example.com to u2\n", stdout.String())
	assert.Equal(t, envTokenWarning, stderr.String())
}
