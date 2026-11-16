package status

import (
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

func TestStatusRun_LoggedIn(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`),
	)

	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{
		Hosts: map[string]*config.HostConfig{
			"app.copia.io": {Token: "abc123", User: "john"},
		},
	}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, stdout, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
	}

	err := StatusRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "app.copia.io")
	assert.Contains(t, stdout.String(), "john")
	assert.Contains(t, stdout.String(), "Token valid")
}

func TestStatusRun_MasksToken(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{}`))

	token := "0123456789abcdef0123456789abcdef01234567"
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": {Token: token, User: "john"},
	}}))

	ios, _, stdout, _ := iostreams.Test()
	err := StatusRun(&StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
	})
	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), token)
	assert.Equal(t, "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: "+strings.Repeat("*", 40)+" (config file)\n  Token valid\n", stdout.String())
}

func TestStatusRun_ShowToken(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{
			name: "all hosts",
			want: "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: abc123 (config file)\n  Token valid\n",
		},
		{
			name: "specific host",
			host: "app.copia.io",
			want: "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: abc123 (config file)\n  Token valid\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{}`))

			hosts := map[string]*config.HostConfig{"app.copia.io": {Token: "abc123", User: "john"}}
			if tt.host != "" {
				hosts["other.copia.io"] = &config.HostConfig{Token: "xyz789", User: "jane"}
			}
			configPath := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, config.Save(configPath, &config.Config{Hosts: hosts}))

			ios, _, stdout, _ := iostreams.Test()
			err := StatusRun(&StatusOptions{
				IO:         ios,
				ConfigPath: configPath,
				HTTPClient: &http.Client{Transport: reg},
				Host:       tt.host,
				ShowToken:  true,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, stdout.String())
			assert.NotContains(t, stdout.String(), "xyz789")
		})
	}
}

func TestStatusRun_ShowTokenWarnsWhenNotTTY(t *testing.T) {
	tests := []struct {
		name      string
		tty       bool
		showToken bool
		wantWarn  bool
	}{
		{name: "show token to a pipe", showToken: true, wantWarn: true},
		{name: "show token to a terminal", tty: true, showToken: true},
		{name: "masked token to a pipe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := setTempConfig(t, map[string]*config.HostConfig{
				"app.copia.io":   {Token: "abc123", User: "john"},
				"other.copia.io": {Token: "xyz789", User: "jane"},
			})

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			err := StatusRun(&StatusOptions{
				IO:         ios,
				ConfigPath: configPath,
				ShowToken:  tt.showToken,
			})
			require.NoError(t, err)

			if !tt.wantWarn {
				assert.Empty(t, stderr.String())
				return
			}
			assert.Equal(t, "warning: printing the token to a non-terminal output; it may end up in a log or a file\n", stderr.String())
			assert.Contains(t, stdout.String(), "Token: abc123 (config file)\n")
			assert.Contains(t, stdout.String(), "Token: xyz789 (config file)\n")
		})
	}
}

func TestStatusRun_SpecificHostMasksToken(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{}`))

	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io":   {Token: "abc123", User: "john"},
		"other.copia.io": {Token: "xyz789", User: "jane"},
	}}))

	ios, _, stdout, _ := iostreams.Test()
	err := StatusRun(&StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "other.copia.io",
	})
	require.NoError(t, err)
	assert.Equal(t, "other.copia.io\n  User: jane\n  Active account: true\n  Token: ****** (config file)\n  Token valid\n", stdout.String())
}

func TestStatusRun_NoHosts(t *testing.T) {
	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{Hosts: map[string]*config.HostConfig{}}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, _, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
	}

	err := StatusRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
}

func TestStatusRun_UnknownHost(t *testing.T) {
	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{
		Hosts: map[string]*config.HostConfig{
			"app.copia.io": {Token: "abc123", User: "john"},
		},
	}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, _, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		Host:       "unknown.example.com",
	}

	err := StatusRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in to unknown.example.com")
}

func TestStatusRun_SpecificHost(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`),
	)

	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{
		Hosts: map[string]*config.HostConfig{
			"app.copia.io":   {Token: "abc123", User: "john"},
			"other.copia.io": {Token: "xyz789", User: "jane"},
		},
	}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, stdout, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
	}

	err := StatusRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "app.copia.io")
	assert.NotContains(t, stdout.String(), "other.copia.io")
}

func TestStatusRun_InvalidToken(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusUnauthorized, `{}`),
	)

	configPath := t.TempDir() + "/config.yml"
	cfg := &config.Config{
		Hosts: map[string]*config.HostConfig{
			"app.copia.io": {Token: "expired", User: "john"},
		},
	}
	require.NoError(t, config.Save(configPath, cfg))

	ios, _, stdout, _ := iostreams.Test()

	opts := &StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
	}

	err := StatusRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Token invalid")
}

func setTempConfig(t *testing.T, hosts map[string]*config.HostConfig) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "copia", "config.yml")
	if hosts != nil {
		require.NoError(t, config.Save(path, &config.Config{Hosts: hosts}))
	}
	return path
}

func runStatusCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdStatus(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdStatus_AllHosts(t *testing.T) {
	setTempConfig(t, map[string]*config.HostConfig{
		"app.copia.io":      {Token: "good", User: "john"},
		"other.example.com": {Token: "bad", User: "jane"},
	})
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		func(req *http.Request) bool { return req.URL.Host == "app.copia.io" && req.URL.Path == "/api/v1/user" },
		httpmock.StringResponse(http.StatusOK, `{"login":"john"}`),
	)
	reg.Register(
		func(req *http.Request) bool {
			return req.URL.Host == "other.example.com" && req.URL.Path == "/api/v1/user"
		},
		httpmock.StringResponse(http.StatusUnauthorized, `{}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	err := runStatusCmd(t, &cmdutil.Factory{IOStreams: ios})
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: **** (config file)\n  Token valid\n")
	assert.Contains(t, out, "other.example.com\n  User: jane\n  Active account: true\n  Token: *** (config file)\n  Token invalid (HTTP 401)\n")
	assert.Len(t, reg.Requests, 2)
}

func TestNewCmdStatus_FactoryHost(t *testing.T) {
	setTempConfig(t, map[string]*config.HostConfig{
		"app.copia.io":      {Token: "good", User: "john"},
		"other.example.com": {Token: "bad", User: "jane"},
	})
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	err := runStatusCmd(t, &cmdutil.Factory{IOStreams: ios, Host: "other.example.com"})
	require.NoError(t, err)
	assert.Equal(t, "other.example.com\n  User: jane\n  Active account: true\n  Token: *** (config file)\n  Token valid\n", stdout.String())
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "other.example.com", reg.Requests[0].URL.Host)
	assert.Equal(t, "token bad", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdStatus_ShowToken(t *testing.T) {
	configPath := setTempConfig(t, map[string]*config.HostConfig{
		"app.copia.io":      {Token: "good", User: "john"},
		"other.example.com": {Token: "bad", User: "jane"},
	})
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	err = runStatusCmd(t, &cmdutil.Factory{IOStreams: ios, Host: "other.example.com"}, "--show-token")
	require.NoError(t, err)
	assert.Equal(t, "other.example.com\n  User: jane\n  Active account: true\n  Token: bad (config file)\n  Token valid\n", stdout.String())
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestNewCmdStatus_ShowTokenFlag(t *testing.T) {
	cmd := NewCmdStatus(&cmdutil.Factory{})
	flag := cmd.Flags().Lookup("show-token")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Display the authentication token", flag.Usage)
}

func TestNewCmdStatus_NoConfigFile(t *testing.T) {
	setTempConfig(t, nil)

	ios, _, _, _ := iostreams.Test()
	err := runStatusCmd(t, &cmdutil.Factory{IOStreams: ios})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in to any Copia instance")
}

func TestStatusRun_LoadError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("hosts: [\n"), 0600))

	ios, _, _, _ := iostreams.Test()
	err := StatusRun(&StatusOptions{IO: ios, ConfigPath: configPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestStatusRun_RequestErrors(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		responder httpmock.Responder
		want      string
	}{
		{
			name: "transport error",
			host: "app.copia.io",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			want: "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: *** (config file)\n  Error: ",
		},
		{
			name: "invalid host",
			host: "bad host",
			want: "bad host (default)\n  User: john\n  Active account: true\n  Token: *** (config file)\n  Error: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/user"), tt.responder)
			}

			configPath := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
				tt.host: {Token: "tok", User: "john"},
			}}))

			ios, _, stdout, _ := iostreams.Test()
			err := StatusRun(&StatusOptions{
				IO:         ios,
				ConfigPath: configPath,
				HTTPClient: &http.Client{Transport: reg},
			})
			require.NoError(t, err)
			assert.Contains(t, stdout.String(), tt.want)
			assert.NotContains(t, stdout.String(), "Token valid")
		})
	}
}

func TestStatusRun_NoHTTPClient(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": {Token: "tok", User: "john"},
	}}))

	ios, _, stdout, _ := iostreams.Test()
	err := StatusRun(&StatusOptions{IO: ios, ConfigPath: configPath})
	require.NoError(t, err)
	assert.Equal(t, "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: *** (config file)\n  Token valid\n", stdout.String())
}

func TestStatusRun_MarksDefaultHost(t *testing.T) {
	tests := []struct {
		name        string
		defaultHost string
		want        string
	}{
		{
			name:        "stored default host",
			defaultHost: "b.example.com",
			want: "a.example.com\n  User: u1\n  Active account: true\n  Token: * (config file)\n  Token valid\n" +
				"b.example.com (default)\n  User: u2\n  Active account: true\n  Token: * (config file)\n  Token valid\n" +
				"c.example.com\n  User: u3\n  Active account: true\n  Token: * (config file)\n  Token valid\n",
		},
		{
			name: "unset default host",
			want: "a.example.com (default)\n  User: u1\n  Active account: true\n  Token: * (config file)\n  Token valid\n" +
				"b.example.com\n  User: u2\n  Active account: true\n  Token: * (config file)\n  Token valid\n" +
				"c.example.com\n  User: u3\n  Active account: true\n  Token: * (config file)\n  Token valid\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, config.Save(configPath, &config.Config{
				DefaultHostName: tt.defaultHost,
				Hosts: map[string]*config.HostConfig{
					"c.example.com": {Token: "c", User: "u3"},
					"a.example.com": {Token: "a", User: "u1"},
					"b.example.com": {Token: "b", User: "u2"},
				},
			}))

			for range 20 {
				ios, _, stdout, _ := iostreams.Test()
				require.NoError(t, StatusRun(&StatusOptions{IO: ios, ConfigPath: configPath}))
				require.Equal(t, tt.want, stdout.String())
			}
		})
	}
}

// failingStore is a keyring that refuses every call with err.
type failingStore struct{ err error }

func (s failingStore) Get(string, string) (string, error) { return "", s.err }
func (s failingStore) Set(string, string, string) error   { return s.err }
func (s failingStore) Delete(string, string) error        { return s.err }

func TestStatusRun_TokenSource(t *testing.T) {
	tests := []struct {
		name      string
		host      *config.HostConfig
		inKeyring bool
		keyring   func() config.TokenStore
		showToken bool
		wantOut   string
		wantCall  bool
	}{
		{
			name:      "keyring",
			host:      &config.HostConfig{User: "john"},
			inKeyring: true,
			keyring:   func() config.TokenStore { return config.NewKeyringStore() },
			wantOut:   "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: ************* (keyring)\n  Token valid\n",
			wantCall:  true,
		},
		{
			name:      "keyring with show-token",
			host:      &config.HostConfig{User: "john"},
			inKeyring: true,
			keyring:   func() config.TokenStore { return config.NewKeyringStore() },
			showToken: true,
			wantOut:   "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: keyring-token (keyring)\n  Token valid\n",
			wantCall:  true,
		},
		{
			name:      "config file before keyring",
			host:      &config.HostConfig{User: "john", Token: "file-token"},
			inKeyring: true,
			keyring:   func() config.TokenStore { return config.NewKeyringStore() },
			wantOut:   "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: ********** (config file)\n  Token valid\n",
			wantCall:  true,
		},
		{
			name:    "no token stored",
			host:    &config.HostConfig{User: "john"},
			keyring: func() config.TokenStore { return config.NewKeyringStore() },
			wantOut: "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: none\n  No token stored\n",
		},
		{
			name:    "keyring unavailable",
			host:    &config.HostConfig{User: "john"},
			keyring: func() config.TokenStore { return failingStore{err: config.ErrKeyringTimeout} },
			wantOut: "app.copia.io (default)\n  User: john\n  Active account: true\n  Token: none\n  Error: timed out waiting for the OS keyring\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			if tt.inKeyring {
				require.NoError(t, keyring.Set("copia-cli:app.copia.io", "john", "keyring-token"))
			}
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.wantCall {
				reg.Register(httpmock.REST("GET", "/api/v1/user"),
					httpmock.StringResponse(http.StatusOK, `{"login":"john"}`))
			}
			configPath := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{"app.copia.io": tt.host}}))

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			err := StatusRun(&StatusOptions{
				IO:         ios,
				ConfigPath: configPath,
				HTTPClient: &http.Client{Transport: reg},
				Keyring:    tt.keyring(),
				ShowToken:  tt.showToken,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			if tt.wantCall {
				require.Len(t, reg.Requests, 1)
				wantToken := "keyring-token"
				if tt.host.Token != "" {
					wantToken = tt.host.Token
				}
				assert.Equal(t, "token "+wantToken, reg.Requests[0].Header.Get("Authorization"))
			}
		})
	}
}

func TestNewCmdStatus_Keyring(t *testing.T) {
	keyring.MockInit()
	setTempConfig(t, map[string]*config.HostConfig{"app.copia.io": {User: "john"}})
	require.NoError(t, keyring.Set("copia-cli:app.copia.io", "john", "keyring-token"))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"john"}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runStatusCmd(t, &cmdutil.Factory{IOStreams: ios, Keyring: config.NewKeyringStore()}))
	assert.Contains(t, stdout.String(), "  Token: ************* (keyring)\n  Token valid\n")
}

func TestStatusRun_SeveralAccounts(t *testing.T) {
	keyring.MockInit()
	require.NoError(t, keyring.Set("copia-cli:app.copia.io", "joe", "keyring-token"))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	// Stubs are not used up, so one responder answers both accounts.
	reg.Register(httpmock.REST("GET", "/api/v1/user"), func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") == "token file" {
			return httpmock.StringResponse(http.StatusUnauthorized, `{}`)(req)
		}
		return httpmock.StringResponse(http.StatusOK, `{}`)(req)
	})

	configPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, config.Save(configPath, &config.Config{Hosts: map[string]*config.HostConfig{
		"app.copia.io": {User: "joe", Users: map[string]*config.UserConfig{"john": {Token: "file"}, "joe": {}, "jane": {}}},
	}}))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, StatusRun(&StatusOptions{
		IO:         ios,
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: reg},
		Keyring:    config.NewKeyringStore(),
	}))
	assert.Equal(t, "app.copia.io (default)\n"+
		"  User: joe\n  Active account: true\n  Token: ************* (keyring)\n  Token valid\n"+
		"  User: jane\n  Active account: false\n  Token: none\n  No token stored\n"+
		"  User: john\n  Active account: false\n  Token: **** (config file)\n  Token invalid (HTTP 401)\n",
		stdout.String())
	require.Len(t, reg.Requests, 2)
	assert.Equal(t, "token keyring-token", reg.Requests[0].Header.Get("Authorization"))
	assert.Equal(t, "token file", reg.Requests[1].Header.Get("Authorization"))
}
