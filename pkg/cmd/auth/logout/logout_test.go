package logout

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
		rawConfig string
		factHost  string
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
			wantOut:  "Logged out of other.example.com\n",
			wantLeft: []string{"app.copia.io"},
		},
		{
			name: "factory host",
			hosts: map[string]*config.HostConfig{
				"app.copia.io":      {Token: "a", User: "john"},
				"other.example.com": {Token: "b", User: "jane"},
			},
			factHost: "app.copia.io",
			wantOut:  "Logged out of app.copia.io\n",
			wantLeft: []string{"other.example.com"},
		},
		{
			name: "default host from config",
			hosts: map[string]*config.HostConfig{
				"app.copia.io": {Token: "a", User: "john"},
			},
			wantOut:  "Logged out of app.copia.io\n",
			wantLeft: []string{},
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
				require.NoError(t, config.Save(configPath, &config.Config{Hosts: tt.hosts}))
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
