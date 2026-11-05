package config

import (
	"os"
	"path/filepath"
	"runtime"
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
	assert.Equal(t, "abc123", loaded.Hosts["app.copia.io"].Users["john"].Token)
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
	assert.Equal(t, "t1", loaded.Hosts["app.copia.io"].Users["u1"].Token)
	assert.Equal(t, "t2", loaded.Hosts["on-prem.corp.com"].Users["u2"].Token)
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

func TestConfig_DefaultHost_ThreeHosts(t *testing.T) {
	tests := []struct {
		name        string
		defaultHost string
		want        string
	}{
		{name: "stored default host", defaultHost: "b.example.com", want: "b.example.com"},
		{name: "unset default host", want: "a.example.com"},
		{name: "stale default host", defaultHost: "gone.example.com", want: "a.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A map's order changes between iterations, so ask often.
			for range 50 {
				cfg := &Config{
					DefaultHostName: tt.defaultHost,
					Hosts: map[string]*HostConfig{
						"c.example.com": {User: "u3"},
						"a.example.com": {User: "u1"},
						"b.example.com": {User: "u2"},
					},
				}
				host, hc := cfg.DefaultHost()
				require.Equal(t, tt.want, host)
				require.Same(t, cfg.Hosts[tt.want], hc)
			}
		})
	}
}

func TestConfig_RemoveHost(t *testing.T) {
	tests := []struct {
		name        string
		hosts       []string
		defaultHost string
		remove      string
		wantDefault string
	}{
		{name: "default host moves to the first remaining", hosts: []string{"c.example.com", "a.example.com", "b.example.com"}, defaultHost: "a.example.com", remove: "a.example.com", wantDefault: "b.example.com"},
		{name: "other host keeps the default", hosts: []string{"c.example.com", "a.example.com", "b.example.com"}, defaultHost: "c.example.com", remove: "a.example.com", wantDefault: "c.example.com"},
		{name: "last host clears the default", hosts: []string{"a.example.com"}, defaultHost: "a.example.com", remove: "a.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DefaultHostName: tt.defaultHost, Hosts: map[string]*HostConfig{}}
			for _, h := range tt.hosts {
				cfg.Hosts[h] = &HostConfig{}
			}
			cfg.RemoveHost(tt.remove)
			assert.NotContains(t, cfg.Hosts, tt.remove)
			assert.Equal(t, tt.wantDefault, cfg.DefaultHostName)
		})
	}
}

func TestDefaultHost_RoundTripKeepsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("default_host: b.example.com\nfuture_key: kept\nhosts:\n  a.example.com:\n    user: u1\n  b.example.com:\n    user: u2\n"), 0600))

	cfg, err := Load(path)
	require.NoError(t, err)
	host, _ := cfg.DefaultHost()
	assert.Equal(t, "b.example.com", host)

	cfg.DefaultHostName = "a.example.com"
	require.NoError(t, Save(path, cfg))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "default_host: a.example.com\n")
	assert.Contains(t, string(data), "future_key: kept\n")
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
	assert.Equal(t, "abc", loaded.Hosts["app.copia.io"].Users["john"].Token)
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
		Hosts:   map[string]*HostConfig{"app.copia.io": {User: "u1", Users: map[string]*UserConfig{"u1": {Token: "t1"}}}},
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

func TestSaveAndLoad_Aliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg := &Config{
		Hosts:   map[string]*HostConfig{"app.copia.io": {User: "u1", Users: map[string]*UserConfig{"u1": {Token: "t1"}}}},
		Aliases: map[string]string{"il": "issue list -L 1", "x": "!echo hi"},
	}
	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, cfg, loaded)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "aliases:\n    il: issue list -L 1\n    x: '!echo hi'\n")
}

func TestSave_OmitsUnsetSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "t1", User: "u1"}}}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hosts:\n    app.copia.io:\n        user: u1\n        users:\n            u1:\n                token: t1\n", string(data))
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

func TestSave_RefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yml")
	require.NoError(t, os.WriteFile(target, []byte("keep: me\n"), 0644))
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.Symlink(target, path))

	err := Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "secret"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "not a regular file")

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "keep: me\n", string(data))
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestSave_TightensWritableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	dir := filepath.Join(t.TempDir(), "copia")
	require.NoError(t, os.Mkdir(dir, 0700))
	require.NoError(t, os.Chmod(dir, 0777))

	require.NoError(t, Save(filepath.Join(dir, "config.yml"), &Config{Hosts: map[string]*HostConfig{}}))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestSave_ReplacesWithNew0600File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("hosts: {}\n"), 0600))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(path, 0666))
	}

	require.NoError(t, Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "t1", User: "u1"}}}))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "t1", loaded.Hosts["app.copia.io"].Users["u1"].Token)
	leftovers, err := filepath.Glob(filepath.Join(dir, ".config-*"))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestSave_KeepsOldFileOnError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions that bind the user")
	}
	dir := filepath.Join(t.TempDir(), "copia")
	require.NoError(t, os.Mkdir(dir, 0700))
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "old", User: "u1"}}}))
	require.NoError(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	err := Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "new", User: "u1"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing config")

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "old", loaded.Hosts["app.copia.io"].Users["u1"].Token)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestSave_RefusesReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions that bind the user")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("hosts: {}\n"), 0400))

	err := Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "t1"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing config")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hosts: {}\n", string(data))
	leftovers, err := filepath.Glob(filepath.Join(dir, ".config-*"))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestLoad_OldShapeIsOneAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("hosts:\n  app.copia.io:\n    token: secret-1\n    user: john\n    git_protocol: ssh\n"), 0600))

	cfg, err := Load(path)
	require.NoError(t, err)
	hc := cfg.Hosts["app.copia.io"]
	assert.Equal(t, "john", hc.User)
	assert.Equal(t, []string{"john"}, hc.Logins())
	assert.Equal(t, "secret-1", hc.Users["john"].Token)
	assert.Empty(t, hc.Token)
}

func TestSave_RewritesOldShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("future_key: kept\nhosts:\n  app.copia.io:\n    token: secret-1\n    user: john\n    git_protocol: ssh\n"), 0600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.NoError(t, Save(path, cfg))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hosts:\n    app.copia.io:\n        user: john\n        users:\n            john:\n                token: secret-1\n        git_protocol: ssh\nfuture_key: kept\n", string(data))
}

func TestSave_OldShapeInMemoryLeavesCallerUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	hc := &HostConfig{Token: "secret-1", User: "john"}

	require.NoError(t, Save(path, &Config{Hosts: map[string]*HostConfig{"app.copia.io": hc}}))
	assert.Equal(t, &HostConfig{Token: "secret-1", User: "john"}, hc)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hosts:\n    app.copia.io:\n        user: john\n        users:\n            john:\n                token: secret-1\n", string(data))
}

func TestSaveAndLoad_TwoAccountsOnOneHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("hosts:\n  app.copia.io:\n    user: jane\n    users:\n      john:\n        token: secret-1\n        future_key: kept\n      jane:\n"), 0600))

	cfg, err := Load(path)
	require.NoError(t, err)
	hc := cfg.Hosts["app.copia.io"]
	assert.Equal(t, "jane", hc.User)
	assert.Equal(t, []string{"jane", "john"}, hc.Logins())

	require.NoError(t, Save(path, cfg))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hosts:\n    app.copia.io:\n        user: jane\n        users:\n            jane: {}\n            john:\n                token: secret-1\n                future_key: kept\n", string(data))
}

func TestConfig_RemoveAccount(t *testing.T) {
	tests := []struct {
		name       string
		remove     string
		wantActive string
		wantLogins []string
		wantHost   bool
	}{
		{name: "active account moves to the first remaining", remove: "joe", wantActive: "jane", wantLogins: []string{"jane", "john"}, wantHost: true},
		{name: "other account keeps the active one", remove: "john", wantActive: "joe", wantLogins: []string{"jane", "joe"}, wantHost: true},
		{name: "unknown account changes nothing", remove: "jim", wantActive: "joe", wantLogins: []string{"jane", "joe", "john"}, wantHost: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DefaultHostName: "app.copia.io", Hosts: map[string]*HostConfig{
				"app.copia.io": {User: "joe", Users: map[string]*UserConfig{"john": {}, "joe": {Token: "t1"}, "jane": {}}},
				"b.example.com": {User: "u2", Users: map[string]*UserConfig{"u2": {}}},
			}}
			active := cfg.RemoveAccount("app.copia.io", tt.remove)
			assert.Equal(t, tt.wantActive, active)
			hc := cfg.Hosts["app.copia.io"]
			assert.Equal(t, tt.wantActive, hc.User)
			assert.Equal(t, tt.wantLogins, hc.Logins())
			assert.Equal(t, "app.copia.io", cfg.DefaultHostName)
		})
	}
}

func TestConfig_RemoveAccount_LastAccountRemovesHost(t *testing.T) {
	cfg := &Config{DefaultHostName: "a.example.com", Hosts: map[string]*HostConfig{
		"a.example.com": {Token: "t1", User: "u1"},
		"b.example.com": {User: "u2", Users: map[string]*UserConfig{"u2": {}}},
	}}

	assert.Empty(t, cfg.RemoveAccount("a.example.com", "u1"))
	assert.NotContains(t, cfg.Hosts, "a.example.com")
	assert.Equal(t, "b.example.com", cfg.DefaultHostName)
}

func TestConfig_SwitchAccount(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		user        string
		wantErr     string
		wantActive  string
		wantDefault string
	}{
		{name: "other account of the default host", host: "app.copia.io", user: "jane", wantActive: "jane", wantDefault: "app.copia.io"},
		{name: "account of another host", host: "b.example.com", user: "u2", wantActive: "joe", wantDefault: "b.example.com"},
		{name: "unknown host", host: "c.example.com", user: "u2", wantErr: "not logged in to c.example.com", wantActive: "joe", wantDefault: "app.copia.io"},
		{name: "unknown login", host: "app.copia.io", user: "jim", wantErr: "not logged in to app.copia.io account jim", wantActive: "joe", wantDefault: "app.copia.io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DefaultHostName: "app.copia.io", Hosts: map[string]*HostConfig{
				"app.copia.io":  {User: "joe", Users: map[string]*UserConfig{"joe": {Token: "t1"}, "jane": {}}},
				"b.example.com": {User: "u2", Users: map[string]*UserConfig{"u2": {}}},
			}}
			err := cfg.SwitchAccount(tt.host, tt.user)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantActive, cfg.Hosts["app.copia.io"].User)
			assert.Equal(t, tt.wantDefault, cfg.DefaultHostName)
			assert.Equal(t, "t1", cfg.Hosts["app.copia.io"].Users["joe"].Token)
		})
	}
}

func TestConfig_SwitchAccount_OldShape(t *testing.T) {
	cfg := &Config{Hosts: map[string]*HostConfig{"a.example.com": {Token: "t1", User: "u1"}}}

	require.NoError(t, cfg.SwitchAccount("a.example.com", "u1"))
	assert.Equal(t, "a.example.com", cfg.DefaultHostName)
	assert.Equal(t, "u1", cfg.Hosts["a.example.com"].User)
	assert.Equal(t, "t1", cfg.Hosts["a.example.com"].Users["u1"].Token)
}

func TestConfig_SwitchAccount_LoginlessToken(t *testing.T) {
	// An old-shape token without a login must not be bound to an account it
	// was not issued for.
	cfg := &Config{DefaultHostName: "b.example.com", Hosts: map[string]*HostConfig{
		"a.example.com": {Token: "T", Users: map[string]*UserConfig{"jane": {}}},
		"b.example.com": {User: "u2", Users: map[string]*UserConfig{"u2": {}}},
	}}

	err := cfg.SwitchAccount("a.example.com", "jane")
	require.EqualError(t, err, "the token of a.example.com has no account; run auth login")
	hc := cfg.Hosts["a.example.com"]
	assert.Equal(t, "T", hc.Token)
	assert.Empty(t, hc.User)
	assert.Empty(t, hc.Users["jane"].Token)
	assert.Equal(t, "b.example.com", cfg.DefaultHostName)
}
