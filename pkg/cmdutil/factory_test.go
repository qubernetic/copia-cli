package cmdutil

import (
	"errors"
	"os"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestFactory_ResolveAuth_FlagOverride(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &Factory{
		IOStreams: ios,
		Token:    "flag-token",
		Host:     "flag-host.io",
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{}}, nil
		},
	}

	host, token, err := f.ResolveAuth()
	assert.NoError(t, err)
	assert.Equal(t, "flag-host.io", host)
	assert.Equal(t, "flag-token", token)
}

func TestFactory_ResolveAuth_EnvOverride(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	t.Setenv("COPIA_TOKEN", "env-token")
	t.Setenv("COPIA_HOST", "env-host.io")

	f := &Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{
				"config-host.io": {Token: "config-token"},
			}}, nil
		},
	}

	host, token, err := f.ResolveAuth()
	assert.NoError(t, err)
	assert.Equal(t, "env-host.io", host)
	assert.Equal(t, "env-token", token)
}

func TestFactory_ResolveAuth_FlagBeatsEnv(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	t.Setenv("COPIA_TOKEN", "env-token")
	t.Setenv("COPIA_HOST", "env-host.io")

	f := &Factory{
		IOStreams: ios,
		Token:    "flag-token",
		Host:     "flag-host.io",
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{}}, nil
		},
	}

	host, token, err := f.ResolveAuth()
	assert.NoError(t, err)
	assert.Equal(t, "flag-host.io", host)
	assert.Equal(t, "flag-token", token)
}

func TestFactory_ResolveAuth_ConfigFallback(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return &config.Config{
				Hosts: map[string]*config.HostConfig{
					"app.copia.io": {Token: "config-token", User: "john"},
				},
			}, nil
		},
	}

	host, token, err := f.ResolveAuth()
	assert.NoError(t, err)
	assert.Equal(t, "app.copia.io", host)
	assert.Equal(t, "config-token", token)
}

func TestFactory_ResolveAuth_StoredToken(t *testing.T) {
	tests := []struct {
		name       string
		host       *config.HostConfig
		keyringErr error
		noKeyring  bool
		wantToken  string
		wantErr    string
	}{
		{
			name:      "keyring token",
			host:      &config.HostConfig{User: "john"},
			wantToken: "keyring-token",
		},
		{
			name:      "config file token before keyring",
			host:      &config.HostConfig{User: "john", Token: "file-token"},
			wantToken: "file-token",
		},
		{
			name: "keyring token of the active account",
			host: &config.HostConfig{User: "jane", Users: map[string]*config.UserConfig{
				"john": {Token: "file-token"}, "jane": {},
			}},
			wantToken: "keyring-token-jane",
		},
		{
			name: "config file token of the active account",
			host: &config.HostConfig{User: "john", Users: map[string]*config.UserConfig{
				"john": {Token: "file-token"}, "jane": {},
			}},
			wantToken: "file-token",
		},
		{
			name:      "no keyring store",
			host:      &config.HostConfig{User: "john"},
			noKeyring: true,
			wantErr:   "no token configured for app.copia.io",
		},
		{
			name:       "keyring unavailable",
			host:       &config.HostConfig{User: "john"},
			keyringErr: errors.New("no secret service"),
			wantErr:    "no secret service",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			require.NoError(t, keyring.Set("copia-cli:app.copia.io", "john", "keyring-token"))
			require.NoError(t, keyring.Set("copia-cli:app.copia.io", "jane", "keyring-token-jane"))
			if tt.keyringErr != nil {
				keyring.MockInitWithError(tt.keyringErr)
				t.Cleanup(keyring.MockInit)
			}
			ios, _, _, _ := iostreams.Test()
			f := &Factory{
				IOStreams: ios,
				Config: func() (*config.Config, error) {
					return &config.Config{Hosts: map[string]*config.HostConfig{"app.copia.io": tt.host}}, nil
				},
			}
			if !tt.noKeyring {
				f.Keyring = config.NewKeyringStore()
			}

			host, token, err := f.ResolveAuth()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "app.copia.io", host)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

// TestFactory_ResolveAuth_EnvTokenOnlyForNamedHost pins that COPIA_TOKEN is
// sent only to the host --host or COPIA_HOST names, never to a host the
// config picks, and that its value appears in no output or error.
func TestFactory_ResolveAuth_EnvTokenOnlyForNamedHost(t *testing.T) {
	const envToken = "env-secret-token"
	const warning = "warning: COPIA_TOKEN is ignored because no host is named; set COPIA_HOST (or --host) to the host the token belongs to\n"
	const hint = "COPIA_TOKEN is ignored because no host is named; set COPIA_HOST (or --host) to the host the token belongs to"
	twoHosts := map[string]*config.HostConfig{
		"a.example.com": {Token: "token-a"},
		"b.example.com": {Token: "token-b"},
	}
	tests := []struct {
		name        string
		flagHost    string
		flagToken   string
		envHost     string
		defaultHost string
		hosts       map[string]*config.HostConfig
		wantHost    string
		wantToken   string
		wantWarning bool
		wantErr     string
	}{
		{
			name:      "COPIA_HOST names the host",
			envHost:   "b.example.com",
			hosts:     twoHosts,
			wantHost:  "b.example.com",
			wantToken: envToken,
		},
		{
			name:      "COPIA_HOST names a host without a stored token",
			envHost:   "new.example.com",
			hosts:     twoHosts,
			wantHost:  "new.example.com",
			wantToken: envToken,
		},
		{
			name:      "--host names the host",
			flagHost:  "b.example.com",
			hosts:     twoHosts,
			wantHost:  "b.example.com",
			wantToken: envToken,
		},
		{
			name:        "host from default_host",
			defaultHost: "b.example.com",
			hosts:       twoHosts,
			wantHost:    "b.example.com",
			wantToken:   "token-b",
			wantWarning: true,
		},
		{
			name:        "host from the alphabetical fallback",
			hosts:       twoHosts,
			wantHost:    "a.example.com",
			wantToken:   "token-a",
			wantWarning: true,
		},
		{
			name:        "config host without a stored token",
			defaultHost: "a.example.com",
			hosts:       map[string]*config.HostConfig{"a.example.com": {}},
			wantErr:     "no token configured for a.example.com. Run 'copia auth login' first. " + hint,
		},
		{
			name:    "no host anywhere",
			hosts:   map[string]*config.HostConfig{},
			wantErr: "no host configured. Run 'copia auth login' first. " + hint,
		},
		{
			name:      "--token beats COPIA_TOKEN with COPIA_HOST",
			flagToken: "flag-token",
			envHost:   "b.example.com",
			hosts:     twoHosts,
			wantHost:  "b.example.com",
			wantToken: "flag-token",
		},
		{
			name:      "--token beats COPIA_TOKEN with --host",
			flagToken: "flag-token",
			flagHost:  "b.example.com",
			hosts:     twoHosts,
			wantHost:  "b.example.com",
			wantToken: "flag-token",
		},
		{
			name:        "--token beats the stored token of the config host",
			flagToken:   "flag-token",
			defaultHost: "b.example.com",
			hosts:       twoHosts,
			wantHost:    "b.example.com",
			wantToken:   "flag-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			t.Setenv("COPIA_TOKEN", envToken)
			t.Setenv("COPIA_HOST", tt.envHost)
			ios, _, stdout, stderr := iostreams.Test()
			f := &Factory{
				IOStreams: ios,
				Host:      tt.flagHost,
				Token:     tt.flagToken,
				Keyring:   config.NewKeyringStore(),
				Config: func() (*config.Config, error) {
					return &config.Config{DefaultHostName: tt.defaultHost, Hosts: tt.hosts}, nil
				},
			}

			for range 3 {
				host, token, err := f.ResolveAuth()
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					assert.NotContains(t, err.Error(), envToken)
					continue
				}
				require.NoError(t, err)
				assert.Equal(t, tt.wantHost, host)
				assert.Equal(t, tt.wantToken, token)
			}

			if tt.wantWarning {
				assert.Equal(t, warning, stderr.String())
			} else {
				assert.Empty(t, stderr.String())
			}
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), envToken)
		})
	}
}

func TestFactory_ResolveAuth_NoConfig(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{}}, nil
		},
	}

	_, _, err := f.ResolveAuth()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no host configured")
}

func TestFactory_ResolveRepo_FlagOverride(t *testing.T) {
	f := &Factory{
		Repo: "flag-org/flag-repo",
		BaseRepo: func() (string, string, error) {
			return "git-org", "git-repo", nil
		},
	}

	owner, repo, err := f.ResolveRepo()
	assert.NoError(t, err)
	assert.Equal(t, "flag-org", owner)
	assert.Equal(t, "flag-repo", repo)
}

func TestFactory_ResolveRepo_BaseRepoFallback(t *testing.T) {
	f := &Factory{
		BaseRepo: func() (string, string, error) {
			return "git-org", "git-repo", nil
		},
	}

	owner, repo, err := f.ResolveRepo()
	assert.NoError(t, err)
	assert.Equal(t, "git-org", owner)
	assert.Equal(t, "git-repo", repo)
}

func TestFactory_ResolveRepo_NoRepoNoBaseRepo(t *testing.T) {
	f := &Factory{}

	_, _, err := f.ResolveRepo()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

func TestFactory_ResolveRepo_InvalidFormat(t *testing.T) {
	f := &Factory{
		Repo: "invalid-no-slash",
	}

	_, _, err := f.ResolveRepo()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected owner/repo format")
}

func TestValidateLimit_Valid(t *testing.T) {
	assert.NoError(t, ValidateLimit(1))
	assert.NoError(t, ValidateLimit(100))
}

func TestValidateLimit_Zero(t *testing.T) {
	err := ValidateLimit(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")
}

func TestValidateLimit_Negative(t *testing.T) {
	err := ValidateLimit(-1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")
}

func TestParseRemoteURL_HTTPS(t *testing.T) {
	owner, repo, err := ParseRemoteURL("https://app.copia.io/my-org/my-repo.git")
	assert.NoError(t, err)
	assert.Equal(t, "my-org", owner)
	assert.Equal(t, "my-repo", repo)
}

func TestParseRemoteURL_HTTPS_NoGitSuffix(t *testing.T) {
	owner, repo, err := ParseRemoteURL("https://app.copia.io/my-org/my-repo")
	assert.NoError(t, err)
	assert.Equal(t, "my-org", owner)
	assert.Equal(t, "my-repo", repo)
}

func TestParseRemoteURL_SSH(t *testing.T) {
	owner, repo, err := ParseRemoteURL("git@app.copia.io:my-org/my-repo.git")
	assert.NoError(t, err)
	assert.Equal(t, "my-org", owner)
	assert.Equal(t, "my-repo", repo)
}

func TestParseRemoteURL_Invalid(t *testing.T) {
	_, _, err := ParseRemoteURL("not-a-url")
	assert.Error(t, err)
}

func TestFactory_ResolveHost(t *testing.T) {
	configWith := func(hosts map[string]*config.HostConfig) func() (*config.Config, error) {
		return func() (*config.Config, error) { return &config.Config{Hosts: hosts}, nil }
	}

	tests := []struct {
		name     string
		flagHost string
		envHost  string
		hosts    map[string]*config.HostConfig
		wantHost string
		wantErr  string
	}{
		{
			name:     "flag beats env and config",
			flagHost: "flag-host.io",
			envHost:  "env-host.io",
			hosts:    map[string]*config.HostConfig{"config-host.io": {Token: "t"}},
			wantHost: "flag-host.io",
		},
		{
			name:     "env beats config",
			envHost:  "env-host.io",
			hosts:    map[string]*config.HostConfig{"config-host.io": {Token: "t"}},
			wantHost: "env-host.io",
		},
		{
			name:     "config host needs no token",
			hosts:    map[string]*config.HostConfig{"config-host.io": {}},
			wantHost: "config-host.io",
		},
		{
			name:    "no host anywhere",
			hosts:   map[string]*config.HostConfig{},
			wantErr: "no host configured. Run 'copia auth login' first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_HOST", tt.envHost)
			t.Setenv("COPIA_TOKEN", "")
			ios, _, _, _ := iostreams.Test()
			f := &Factory{IOStreams: ios, Host: tt.flagHost, Config: configWith(tt.hosts)}

			host, err := f.ResolveHost()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantHost, host)
		})
	}
}

func TestFactory_ResolveHost_ConfigError(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &Factory{IOStreams: ios, Config: func() (*config.Config, error) {
		return nil, errors.New("broken config")
	}}

	_, err := f.ResolveHost()
	assert.EqualError(t, err, "broken config")
}

func TestFactory_ConfigValue(t *testing.T) {
	tests := []struct {
		name   string
		config func() (*config.Config, error)
		key    string
		want   string
	}{
		{
			name:   "set key",
			config: func() (*config.Config, error) { return &config.Config{Browser: "firefox"}, nil },
			key:    "browser",
			want:   "firefox",
		},
		{
			name:   "default of an unset key",
			config: func() (*config.Config, error) { return &config.Config{}, nil },
			key:    "output",
			want:   "table",
		},
		{
			name:   "unknown key",
			config: func() (*config.Config, error) { return &config.Config{}, nil },
			key:    "hosts",
		},
		{
			name:   "unreadable config",
			config: func() (*config.Config, error) { return nil, errors.New("parsing config: bad yaml") },
			key:    "browser",
		},
		{
			name: "no config loader",
			key:  "browser",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Factory{Config: tt.config}
			assert.Equal(t, tt.want, f.ConfigValue(tt.key))
		})
	}
}

func TestFactory_OutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		config  string
		want    string
		wantErr string
	}{
		{name: "default", want: "table"},
		{name: "config", config: "json", want: "json"},
		{name: "env beats config", env: "table", config: "json", want: "table"},
		{name: "env without config", env: "json", want: "json"},
		{name: "invalid env", env: "yaml", wantErr: `invalid COPIA_OUTPUT "yaml": valid values are table, json`},
		{name: "invalid config", config: "yaml", wantErr: `invalid config key output "yaml": valid values are table, json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_OUTPUT", tt.env)
			f := &Factory{Config: func() (*config.Config, error) { return &config.Config{Output: tt.config}, nil }}
			got, err := f.OutputFormat()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFactory_Pager(t *testing.T) {
	tests := []struct {
		name   string
		env    *string
		config string
		pager  string
		want   string
	}{
		{name: "none"},
		{name: "PAGER", pager: "more", want: "more"},
		{name: "config beats PAGER", config: "less -R", pager: "more", want: "less -R"},
		{name: "env beats config", env: ptr("most"), config: "less -R", pager: "more", want: "most"},
		{name: "empty env turns the pager off", env: ptr(""), config: "less -R", pager: "more", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_PAGER", "")
			if tt.env == nil {
				require.NoError(t, os.Unsetenv("COPIA_PAGER"))
			} else {
				t.Setenv("COPIA_PAGER", *tt.env)
			}
			t.Setenv("PAGER", tt.pager)
			f := &Factory{Config: func() (*config.Config, error) { return &config.Config{Pager: tt.config}, nil }}
			assert.Equal(t, tt.want, f.Pager())
		})
	}
}

func ptr(s string) *string { return &s }

func TestFactory_DefaultHostOfThreeHosts(t *testing.T) {
	tests := []struct {
		name        string
		defaultHost string
		wantHost    string
	}{
		{name: "stored default host", defaultHost: "b.example.com", wantHost: "b.example.com"},
		{name: "unset default host", wantHost: "a.example.com"},
		{name: "stale default host", defaultHost: "gone.example.com", wantHost: "a.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_HOST", "")
			t.Setenv("COPIA_TOKEN", "")
			ios, _, _, _ := iostreams.Test()
			f := &Factory{IOStreams: ios, Config: func() (*config.Config, error) {
				return &config.Config{DefaultHostName: tt.defaultHost, Hosts: map[string]*config.HostConfig{
					"c.example.com": {Token: "token-c"},
					"a.example.com": {Token: "token-a"},
					"b.example.com": {Token: "token-b"},
				}}, nil
			}}

			for range 50 {
				host, err := f.ResolveHost()
				require.NoError(t, err)
				require.Equal(t, tt.wantHost, host)

				host, token, err := f.ResolveAuth()
				require.NoError(t, err)
				require.Equal(t, tt.wantHost, host)
				require.Equal(t, "token-"+tt.wantHost[:1], token)
			}
		})
	}
}
