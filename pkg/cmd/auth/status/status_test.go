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
	assert.Equal(t, "app.copia.io\n  User: john\n  Token: "+strings.Repeat("*", 40)+"\n  Token valid\n", stdout.String())
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

func runStatusCmd(t *testing.T, f *cmdutil.Factory) error {
	t.Helper()
	cmd := NewCmdStatus(f)
	cmd.SetArgs([]string{})
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
	assert.Contains(t, out, "app.copia.io\n  User: john\n  Token: ****\n  Token valid\n")
	assert.Contains(t, out, "other.example.com\n  User: jane\n  Token: ***\n  Token invalid (HTTP 401)\n")
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
	assert.Equal(t, "other.example.com\n  User: jane\n  Token: ***\n  Token valid\n", stdout.String())
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "other.example.com", reg.Requests[0].URL.Host)
	assert.Equal(t, "token bad", reg.Requests[0].Header.Get("Authorization"))
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
			want: "app.copia.io\n  User: john\n  Token: ***\n  Error: ",
		},
		{
			name: "invalid host",
			host: "bad host",
			want: "bad host\n  User: john\n  Token: ***\n  Error: ",
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
	assert.Equal(t, "app.copia.io\n  User: john\n  Token: ***\n  Token valid\n", stdout.String())
}
