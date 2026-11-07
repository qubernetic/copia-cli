package token

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

const (
	fileToken    = "file-token-0123456789"
	keyringToken = "keyring-token-0123456789"
	otherToken   = "other-host-token-0123456789"
	flagToken    = "flag-token-0123456789"
	envToken     = "env-token-0123456789"
	legacyToken  = "legacy-token-0123456789"
)

// allTokens lists every token of the tests; none may reach stderr or an
// error message.
var allTokens = []string{fileToken, keyringToken, otherToken, flagToken, envToken, legacyToken}

// testConfig has app.copia.io (the default host) with the active account
// john, whose token is in the config file, and jane, whose token is in the
// keyring, other.example.com with the account bob, whose token is in the
// config file, and legacy.example.com in the old shape, a token without a
// user.
func testConfig(t *testing.T) func() (*config.Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(path, &config.Config{
		DefaultHostName: "app.copia.io",
		Hosts: map[string]*config.HostConfig{
			"app.copia.io": {User: "john", Users: map[string]*config.UserConfig{
				"john": {Token: fileToken},
				"jane": {},
			}},
			"other.example.com": {User: "bob", Users: map[string]*config.UserConfig{
				"bob": {Token: otherToken},
			}},
			"empty.example.com": {User: "ann", Users: map[string]*config.UserConfig{
				"ann": {},
			}},
			"legacy.example.com": {Token: legacyToken},
		},
	}))
	return func() (*config.Config, error) { return config.Load(path) }
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdToken(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdToken(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		flagToken  string // the root command's --token
		flagHost   string // the root command's --host
		envHost    string
		envToken   string
		want       string
		wantStderr string
		wantErr    string
	}{
		{name: "stored token of the active account", want: fileToken},
		{name: "token flag", flagToken: flagToken, want: flagToken},
		{name: "token flag before COPIA_TOKEN", flagToken: flagToken, envHost: "app.copia.io", envToken: envToken, want: flagToken},
		{name: "COPIA_TOKEN with COPIA_HOST", envHost: "app.copia.io", envToken: envToken, want: envToken},
		{name: "COPIA_TOKEN with the host flag", flagHost: "app.copia.io", envToken: envToken, want: envToken},
		{name: "COPIA_TOKEN with the host flag naming COPIA_HOST", flagHost: "APP.copia.io", envHost: "app.copia.io:443", envToken: envToken, want: envToken},
		{
			name:       "COPIA_TOKEN without a named host",
			envToken:   envToken,
			want:       fileToken,
			wantStderr: "warning: COPIA_TOKEN is ignored because no host is named; set COPIA_HOST (or --host) to the host the token belongs to\n",
		},
		{
			name:       "COPIA_TOKEN with the host flag naming another host",
			flagHost:   "other.example.com",
			envHost:    "app.copia.io",
			envToken:   envToken,
			want:       otherToken,
			wantStderr: "warning: COPIA_TOKEN is ignored because --host names another host than COPIA_HOST; the token is sent only to the host COPIA_HOST names\n",
		},
		{name: "host flag", flagHost: "other.example.com", want: otherToken},
		{name: "keyring", args: []string{"--user", "jane"}, want: keyringToken},
		{name: "user in the config file", args: []string{"-u", "john"}, want: fileToken},
		{name: "user on the host flag", args: []string{"--user", "bob"}, flagHost: "other.example.com", want: otherToken},
		{name: "user ignores the token flag", args: []string{"--user", "john"}, flagToken: flagToken, want: fileToken},
		{name: "user ignores COPIA_TOKEN", args: []string{"--user", "john"}, envHost: "app.copia.io", envToken: envToken, want: fileToken},
		{name: "unknown user", args: []string{"--user", "bob"}, wantErr: "no token found for app.copia.io account bob"},
		{name: "user on the host in another case", args: []string{"--user", "john"}, flagHost: "APP.copia.io", wantErr: "no token found for APP.copia.io account john"},
		{name: "user on the host with the default port", args: []string{"--user", "john"}, flagHost: "app.copia.io:443", wantErr: "no token found for app.copia.io:443 account john"},
		{name: "user on a host of the old shape", args: []string{"--user", "john"}, flagHost: "legacy.example.com", wantErr: "no token found for legacy.example.com account john"},
		{name: "user on an unknown host", args: []string{"--user", "bob"}, flagHost: "new.example.com", wantErr: "no token found for new.example.com account bob"},
		{name: "no token", flagHost: "empty.example.com", wantErr: "no token found for empty.example.com"},
		{name: "unknown host", flagHost: "new.example.com", wantErr: "no token found for new.example.com"},
		{name: "arguments", args: []string{"app.copia.io"}, wantErr: `unknown command "app.copia.io" for "token"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_HOST", tt.envHost)
			t.Setenv("COPIA_TOKEN", tt.envToken)
			keyring.MockInit()
			require.NoError(t, keyring.Set("copia-cli:app.copia.io", "jane", keyringToken))
			ios, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config:    testConfig(t),
				Keyring:   config.NewKeyringStore(),
				Token:     tt.flagToken,
				Host:      tt.flagHost,
			}

			err := runCmd(f, tt.args...)

			assert.Equal(t, tt.wantStderr, stderr.String())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, stdout.String())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want+"\n", stdout.String())
		})
	}
}

func TestNewCmdToken_FileStorageOnly(t *testing.T) {
	keyring.MockInit()
	require.NoError(t, keyring.Set("copia-cli:app.copia.io", "jane", keyringToken))
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	ios, _, stdout, _ := iostreams.Test()
	// Without a keyring only the config file is read.
	f := &cmdutil.Factory{IOStreams: ios, Config: testConfig(t)}

	require.EqualError(t, runCmd(f, "--user", "jane"), "no token found for app.copia.io account jane")
	assert.Empty(t, stdout.String())
	require.NoError(t, runCmd(f))
	assert.Equal(t, fileToken+"\n", stdout.String())
}

func TestNewCmdToken_KeyringError(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	keyring.MockInitWithError(errors.New("backend failed"))
	t.Cleanup(keyring.MockInit)

	for _, args := range [][]string{{"--user", "jane"}, nil} {
		ios, _, stdout, stderr := iostreams.Test()
		cfg := testConfig(t)
		f := &cmdutil.Factory{IOStreams: ios, Config: cfg, Keyring: config.NewKeyringStore()}
		if args == nil {
			// Make jane the active account, whose token is in the keyring.
			c, err := cfg()
			require.NoError(t, err)
			require.NoError(t, c.SwitchAccount("app.copia.io", "jane"))
			f.Config = func() (*config.Config, error) { return c, nil }
		}

		err := runCmd(f, args...)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading the token for app.copia.io")
		assert.Contains(t, err.Error(), "OS keyring")
		assert.Empty(t, stdout.String())
		for _, tok := range allTokens {
			assert.NotContains(t, err.Error(), tok)
			assert.NotContains(t, stderr.String(), tok)
		}
	}
}

func TestNewCmdToken_NoHost(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) { return &config.Config{}, nil }}

	for _, args := range [][]string{nil, {"--user", "john"}} {
		err := runCmd(f, args...)
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "no host configured"), err.Error())
		assert.Empty(t, stdout.String())
	}
}

func TestNewCmdToken_ConfigError(t *testing.T) {
	t.Setenv("COPIA_HOST", "app.copia.io")
	t.Setenv("COPIA_TOKEN", "")
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) { return nil, errors.New("bad config") }}

	for _, args := range [][]string{nil, {"--user", "john"}} {
		require.EqualError(t, runCmd(f, args...), "bad config")
		assert.Empty(t, stdout.String())
	}
}
