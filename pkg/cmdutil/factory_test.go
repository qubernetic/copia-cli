package cmdutil

import (
	"errors"
	"os"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
