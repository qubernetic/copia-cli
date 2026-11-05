package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(filepath.Join(dir, "config.yml"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Hosts)
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	cfg := &Config{
		Hosts: map[string]*HostConfig{
			"app.copia.io": {
				Token: "abc123",
				User:  "john",
			},
		},
	}

	err := Save(path, cfg)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "abc123", loaded.Hosts["app.copia.io"].Token)
	assert.Equal(t, "john", loaded.Hosts["app.copia.io"].User)
}

func TestSaveAndLoad_MultipleHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	cfg := &Config{
		Hosts: map[string]*HostConfig{
			"app.copia.io":     {Token: "t1", User: "u1"},
			"on-prem.corp.com": {Token: "t2", User: "u2"},
		},
	}

	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Hosts, 2)
	assert.Equal(t, "t1", loaded.Hosts["app.copia.io"].Token)
	assert.Equal(t, "t2", loaded.Hosts["on-prem.corp.com"].Token)
}

func TestConfig_DefaultHost(t *testing.T) {
	cfg := &Config{
		Hosts: map[string]*HostConfig{
			"app.copia.io": {Token: "t1", User: "u1"},
		},
	}
	host, hc := cfg.DefaultHost()
	assert.Equal(t, "app.copia.io", host)
	assert.Equal(t, "t1", hc.Token)
}

func TestConfig_DefaultHost_Empty(t *testing.T) {
	cfg := &Config{Hosts: map[string]*HostConfig{}}
	host, hc := cfg.DefaultHost()
	assert.Empty(t, host)
	assert.Nil(t, hc)
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "config.yml")

	cfg := &Config{
		Hosts: map[string]*HostConfig{
			"app.copia.io": {Token: "abc", User: "john"},
		},
	}

	err := Save(path, cfg)
	require.NoError(t, err)

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "abc", loaded.Hosts["app.copia.io"].Token)
}

func TestDefaultPath_RespectsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	path := DefaultPath()
	assert.Equal(t, "/custom/config/copia/config.yml", path)
}

func TestConfig_Get(t *testing.T) {
	cfg := &Config{Hosts: map[string]*HostConfig{}, Browser: "firefox"}

	tests := []struct {
		key     string
		want    string
		wantErr string
	}{
		{key: "output", want: "table"},
		{key: "browser", want: "firefox"},
		{key: "pager", want: ""},
		{key: "editor", want: ""},
		{key: "hosts", wantErr: `unknown config key "hosts"; known keys: output, browser, pager, editor`},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := cfg.Get(tt.key)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfig_Set(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr string
	}{
		{key: "output", value: "json"},
		{key: "output", value: "table"},
		{key: "output", value: "yaml", wantErr: `failed to set "output" to "yaml": valid values are table, json`},
		{key: "browser", value: "firefox --new-tab"},
		{key: "pager", value: "less -R"},
		{key: "editor", value: "vim"},
		{key: "editor", value: ""},
		{key: "token", value: "x", wantErr: `unknown config key "token"; known keys: output, browser, pager, editor`},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			cfg := &Config{Hosts: map[string]*HostConfig{}}
			err := cfg.Set(tt.key, tt.value)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Equal(t, &Config{Hosts: map[string]*HostConfig{}}, cfg)
				return
			}
			require.NoError(t, err)
			got, err := cfg.Get(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestSaveAndLoad_Settings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg := &Config{
		Hosts:   map[string]*HostConfig{"app.copia.io": {Token: "t1", User: "u1"}},
		Output:  "json",
		Browser: "firefox",
		Pager:   "less",
		Editor:  "vim",
	}
	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, cfg, loaded)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "output: json\n")
}

func TestSave_OmitsUnsetSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "t1", User: "u1"}}}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hosts:\n    app.copia.io:\n        token: t1\n        user: u1\n", string(data))
}

func TestOptions(t *testing.T) {
	var keys []string
	for _, o := range Options {
		keys = append(keys, o.Key)
		assert.NotEmpty(t, o.Description, o.Key)
	}
	assert.Equal(t, []string{"output", "browser", "pager", "editor"}, keys)
}

func TestSave_RestoresMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("hosts: {}\n"), 0644))

	require.NoError(t, Save(path, &Config{Hosts: map[string]*HostConfig{}, Output: "json"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}
