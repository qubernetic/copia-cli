package gitcredential

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// account returns a host entry whose active account is login, with token
// stored in the config file; an empty token leaves it to the keyring.
func account(login, token string) *config.HostConfig {
	return &config.HostConfig{User: login, Users: map[string]*config.UserConfig{login: {Token: token}}}
}

func TestHelperRun(t *testing.T) {
	twoAccounts := &config.HostConfig{User: "jane", Users: map[string]*config.UserConfig{
		"john": {Token: "john-token"},
		"jane": {Token: "jane-token"},
	}}
	tests := []struct {
		name      string
		operation string
		hosts     map[string]*config.HostConfig
		keyring   map[string]string // login on app.copia.io -> token
		input     string
		want      string
		wantErr   string
	}{
		{
			name:      "known host from the config file",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\n\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "known host from the keyring",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "")},
			keyring:   map[string]string{"john": "keyring-token"},
			input:     "protocol=https\nhost=app.copia.io\n",
			want:      "username=john\npassword=keyring-token\n",
		},
		{
			name:      "host in another case",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=App.Copia.IO\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "path in the request",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\npath=my-org/my-repo.git\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "url line",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "url=https://app.copia.io/my-org/my-repo.git\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "several accounts answer with the active one",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": twoAccounts},
			input:     "protocol=https\nhost=app.copia.io\n",
			want:      "username=jane\npassword=jane-token\n",
		},
		{
			name:      "several hosts answer with the requested one",
			operation: "get",
			hosts: map[string]*config.HostConfig{
				"app.copia.io":           account("john", "copia-token"),
				"copia.example.com:8443": account("jane", "example-token"),
			},
			input: "protocol=https\nhost=copia.example.com:8443\n",
			want:  "username=jane\npassword=example-token\n",
		},
		{
			name:      "explicit port 443",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io:443\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "username of the active account",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\nusername=John\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "token without a login",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": {Token: "old-token"}},
			input:     "protocol=https\nhost=app.copia.io\n",
			want:      "username=token\npassword=old-token\n",
		},
		{
			name:      "another username",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\nusername=jane\n",
		},
		{
			name:      "unknown host",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=evil.example.com\n",
		},
		{
			name:      "suffix of a known host",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io.evil.example.com\n",
		},
		{
			name:      "user name in the host",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=evil.example.com@app.copia.io\n",
		},
		{
			name:      "http",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=http\nhost=app.copia.io\n",
		},
		{
			name:      "no protocol",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "host=app.copia.io\n",
		},
		{
			name:      "another port",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io:8443\n",
		},
		{
			name:      "missing custom port",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"copia.example.com:8443": account("john", "file-token")},
			input:     "protocol=https\nhost=copia.example.com\n",
		},
		{
			name:      "lines after the empty line",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\n\nhost=app.copia.io\n",
		},
		{
			name:      "line without a value",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\ncapability\nhost=app.copia.io\n",
			want:      "username=john\npassword=file-token\n",
		},
		{
			name:      "account without a token",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "")},
			input:     "protocol=https\nhost=app.copia.io\n",
		},
		{
			name:      "host without an account",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": {}},
			input:     "protocol=https\nhost=app.copia.io\n",
		},
		{
			name:      "no hosts",
			operation: "get",
			input:     "protocol=https\nhost=app.copia.io\n",
		},
		{
			name:      "invalid configured host is skipped",
			operation: "get",
			hosts: map[string]*config.HostConfig{
				"*.copia.io":   account("jane", "wildcard-token"),
				"app.copia.io": account("john", "file-token"),
			},
			input: "protocol=https\nhost=app.copia.io\n",
			want:  "username=john\npassword=file-token\n",
		},
		{
			name:      "token with a newline",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "line\nhost=evil.example.com")},
			input:     "protocol=https\nhost=app.copia.io\n",
			wantErr:   "invalid token for app.copia.io: it may contain only printable ASCII characters",
		},
		{
			name:      "login with a newline",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john\npassword=x", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\n",
			wantErr:   "invalid login for app.copia.io: it may contain only printable ASCII characters",
		},
		{
			name:      "invalid url line",
			operation: "get",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "url=https://app.copia.io:port/\n",
			wantErr:   "git-credential: invalid url in the request",
		},
		{
			name:      "store",
			operation: "store",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\nusername=john\npassword=other-token\n",
		},
		{
			name:      "erase",
			operation: "erase",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			input:     "protocol=https\nhost=app.copia.io\nusername=john\npassword=file-token\n",
		},
		{
			name:      "unknown operation",
			operation: "list",
			hosts:     map[string]*config.HostConfig{"app.copia.io": account("john", "file-token")},
			wantErr:   `git-credential: "list" operation not supported`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			store := config.NewKeyringStore()
			for login, token := range tt.keyring {
				require.NoError(t, store.Set("app.copia.io", login, token))
			}
			hosts := tt.hosts
			if hosts == nil {
				hosts = map[string]*config.HostConfig{}
			}
			cfg := &config.Config{Hosts: hosts}

			ios, stdin, stdout, stderr := iostreams.Test()
			stdin.WriteString(tt.input)

			err := helperRun(&CredentialOptions{
				IO:        ios,
				Operation: tt.operation,
				Config:    func() (*config.Config, error) { return cfg, nil },
				Keyring:   store,
			})
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

// failingStore is a keyring that refuses every call with err.
type failingStore struct{ err error }

func (s failingStore) Get(string, string) (string, error) { return "", s.err }
func (s failingStore) Set(string, string, string) error   { return s.err }
func (s failingStore) Delete(string, string) error        { return s.err }

func TestHelperRun_KeyringError(t *testing.T) {
	cfg := &config.Config{Hosts: map[string]*config.HostConfig{"app.copia.io": account("john", "")}}
	ios, stdin, stdout, _ := iostreams.Test()
	stdin.WriteString("protocol=https\nhost=app.copia.io\n")

	err := helperRun(&CredentialOptions{
		IO:        ios,
		Operation: "get",
		Config:    func() (*config.Config, error) { return cfg, nil },
		Keyring:   failingStore{err: config.ErrKeyringTimeout},
	})
	assert.EqualError(t, err, "reading the token for app.copia.io: timed out waiting for the OS keyring")
	assert.Empty(t, stdout.String())
}

func TestHelperRun_ConfigError(t *testing.T) {
	ios, stdin, stdout, _ := iostreams.Test()
	stdin.WriteString("protocol=https\nhost=app.copia.io\n")

	err := helperRun(&CredentialOptions{
		IO:        ios,
		Operation: "get",
		Config:    func() (*config.Config, error) { return nil, errors.New("parsing config: bad yaml") },
	})
	assert.EqualError(t, err, "parsing config: bad yaml")
	assert.Empty(t, stdout.String())
}

func TestNewCmdCredential(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": account("john", "file-token"),
	}}))
	// git asks for each host separately, so neither COPIA_TOKEN nor --token
	// may answer for the host.
	t.Setenv("COPIA_TOKEN", "env-token")
	t.Setenv("COPIA_HOST", "app.copia.io")

	ios, stdin, stdout, stderr := iostreams.Test()
	stdin.WriteString("protocol=https\nhost=app.copia.io\n")
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return config.Load(configPath) },
		Token:     "flag-token",
		Host:      "other.example.com",
	}

	cmd := NewCmdCredential(f)
	assert.True(t, cmd.Hidden)
	cmd.SetArgs([]string{"get"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "username=john\npassword=file-token\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNewCmdCredential_Args(t *testing.T) {
	for _, args := range [][]string{{}, {"get", "store"}} {
		ios, _, _, _ := iostreams.Test()
		cmd := NewCmdCredential(&cmdutil.Factory{IOStreams: ios})
		cmd.SetArgs(args)
		cmd.SetOut(ios.ErrOut)
		cmd.SetErr(ios.ErrOut)
		assert.Error(t, cmd.Execute())
	}
}
