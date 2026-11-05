// Package config manages the CLI configuration file (~/.config/copia/config.yml),
// including host credentials, default host resolution, and file permissions.
package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// HostConfig stores the accounts of a single Copia/Gitea instance, as gh
// does: Users holds every account by login and User names the active one.
type HostConfig struct {
	// Token is the token of the config shape before several accounts, when
	// it belonged to User. Load and Save move it to Users[User].
	Token string `yaml:"token,omitempty"`
	User  string `yaml:"user"`
	Users map[string]*UserConfig `yaml:"users,omitempty"`
	// Extra keeps the keys this version does not know, so a save writes
	// them back unchanged.
	Extra map[string]any `yaml:",inline"`
}

// UserConfig stores one account of a host. Token is empty when the token
// lives in the OS keyring (see LookupToken).
type UserConfig struct {
	Token string `yaml:"token,omitempty"`
	// Extra keeps the keys this version does not know.
	Extra map[string]any `yaml:",inline"`
}

// Logins returns the logins of the host's accounts in alphabetical order.
func (hc *HostConfig) Logins() []string {
	return slices.Sorted(maps.Keys(hc.migrated().Users))
}

// migrated returns hc in the shape with several accounts: a token of the old
// shape moves to the account of User, and User always has an account. The
// result is a copy whenever it differs, so hc stays as it is.
func (hc *HostConfig) migrated() *HostConfig {
	if hc.User == "" || (hc.Token == "" && hc.Users[hc.User] != nil) {
		return hc
	}
	out := *hc
	out.Users = make(map[string]*UserConfig, len(hc.Users)+1)
	maps.Copy(out.Users, hc.Users)
	account := UserConfig{}
	if old := hc.Users[hc.User]; old != nil {
		account = *old
	}
	if account.Token == "" {
		account.Token = hc.Token
	}
	out.Users[hc.User] = &account
	out.Token = ""
	return &out
}

// Config is the top-level configuration: the hosts and the settings that
// `copia-cli config` reads and writes.
type Config struct {
	// DefaultHostName is the host `auth login` logged in to last; read it
	// through DefaultHost, which also handles an unset or stale value.
	DefaultHostName string                 `yaml:"default_host,omitempty"`
	Hosts           map[string]*HostConfig `yaml:"hosts"`
	Output          string                 `yaml:"output,omitempty"`
	Browser         string                 `yaml:"browser,omitempty"`
	Pager           string                 `yaml:"pager,omitempty"`
	Editor          string                 `yaml:"editor,omitempty"`
	// Aliases maps an alias name to its expansion, as `copia-cli alias`
	// manages them.
	Aliases map[string]string `yaml:"aliases,omitempty"`
	// Extra keeps the top-level keys this version does not know, so a save
	// writes them back unchanged.
	Extra map[string]any `yaml:",inline"`
}

// Option describes one setting key of the config file.
type Option struct {
	Key           string
	Description   string
	DefaultValue  string
	AllowedValues []string
}

// Options lists the setting keys in the order `config list` prints them.
var Options = []Option{
	{
		Key:           "output",
		Description:   "the default output format of the commands that take --json",
		DefaultValue:  "table",
		AllowedValues: []string{"table", "json"},
	},
	{
		Key:         "browser",
		Description: "the web browser to use for opening URLs",
	},
	{
		Key:         "pager",
		Description: "the terminal pager program to send standard output to",
	},
	{
		Key:         "editor",
		Description: "the text editor program to use for authoring text",
	},
}

func (c *Config) field(key string) (*string, *Option, error) {
	fields := map[string]*string{
		"output":  &c.Output,
		"browser": &c.Browser,
		"pager":   &c.Pager,
		"editor":  &c.Editor,
	}
	for i := range Options {
		if Options[i].Key == key {
			return fields[key], &Options[i], nil
		}
	}
	keys := make([]string, len(Options))
	for i, o := range Options {
		keys[i] = o.Key
	}
	return nil, nil, fmt.Errorf("unknown config key %q; known keys: %s", key, strings.Join(keys, ", "))
}

// Get returns the value of a setting key, or its default when it is unset.
func (c *Config) Get(key string) (string, error) {
	value, opt, err := c.field(key)
	if err != nil {
		return "", err
	}
	if *value == "" {
		return opt.DefaultValue, nil
	}
	return *value, nil
}

// Set validates value and stores it under a setting key. An unknown key or
// a value outside the key's allowed values leaves the config unchanged.
func (c *Config) Set(key, value string) error {
	field, opt, err := c.field(key)
	if err != nil {
		return err
	}
	if len(opt.AllowedValues) > 0 && !slices.Contains(opt.AllowedValues, value) {
		return fmt.Errorf("failed to set %q to %q: valid values are %s", key, value, strings.Join(opt.AllowedValues, ", "))
	}
	*field = value
	return nil
}

// DefaultPath returns the default config file path.
func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "copia", "config.yml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "copia", "config.yml")
}

// Load reads a config file. Returns empty config if file does not exist.
func Load(path string) (*Config, error) {
	cfg := &Config{Hosts: map[string]*HostConfig{}}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]*HostConfig{}
	}
	for host, hc := range cfg.Hosts {
		if hc == nil {
			hc = &HostConfig{}
		}
		// An account written as a bare key ("jane:") has no value.
		for login, u := range hc.Users {
			if u == nil {
				hc.Users[login] = &UserConfig{}
			}
		}
		cfg.Hosts[host] = hc.migrated()
	}
	return cfg, nil
}

// migrated returns c with every host in the shape with several accounts,
// leaving c and its hosts unchanged.
func (c *Config) migrated() *Config {
	out := *c
	out.Hosts = make(map[string]*HostConfig, len(c.Hosts))
	for host, hc := range c.Hosts {
		if hc != nil {
			hc = hc.migrated()
		}
		out.Hosts[host] = hc
	}
	return &out
}

// Save writes config to path as a new file with 0600 permissions. It refuses
// a path that is not a regular file (a symlink, for example) and an existing
// file the user cannot write, and tightens a config directory that group or
// others can write to 0700. The new content goes to a temporary file in the
// same directory that replaces path only once it is complete, so a failed
// save leaves the old file as it was.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg.migrated())
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	if err := secureDir(dir); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return fmt.Errorf("writing config: %s is not a regular file", path)
	case err == nil:
		// The rename below would replace a read-only file or one owned by
		// another user, so check first that the user may write it.
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		_ = f.Close()
	case !os.IsNotExist(err):
		return fmt.Errorf("writing config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yml")
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := writeAndClose(tmp, data); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// secureDir drops the group and other write bits of the config directory by
// setting it to 0700. Windows has no such bits, so it is left alone there.
func secureDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("securing config directory: %w", err)
	}
	if info.Mode().Perm()&0022 == 0 {
		return nil
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("securing config directory: %s is writable by group or others: %w", dir, err)
	}
	return nil
}

func writeAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// DefaultHost returns the host named by default_host. When default_host is
// unset or names a host that is no longer configured, it returns the
// alphabetically first host. Returns empty string and nil if no hosts
// configured.
func (c *Config) DefaultHost() (string, *HostConfig) {
	if hc, ok := c.Hosts[c.DefaultHostName]; ok {
		return c.DefaultHostName, hc
	}
	hosts := c.HostNames()
	if len(hosts) == 0 {
		return "", nil
	}
	return hosts[0], c.Hosts[hosts[0]]
}

// HostNames returns the configured hosts in alphabetical order.
func (c *Config) HostNames() []string {
	return slices.Sorted(maps.Keys(c.Hosts))
}

// RemoveHost deletes a host. When it was the default host, default_host
// moves to the alphabetically first remaining host, or is cleared when none
// remains.
func (c *Config) RemoveHost(host string) {
	delete(c.Hosts, host)
	if c.DefaultHostName != host {
		return
	}
	c.DefaultHostName = ""
	if hosts := c.HostNames(); len(hosts) > 0 {
		c.DefaultHostName = hosts[0]
	}
}

// RemoveAccount deletes the account user of host and returns the host's
// active account afterwards. When user was the active account, the
// alphabetically first remaining account becomes active. When no account
// remains, the host is removed as RemoveHost does and the result is empty.
func (c *Config) RemoveAccount(host, user string) string {
	hc := c.Hosts[host]
	if hc == nil {
		return ""
	}
	hc = hc.migrated()
	delete(hc.Users, user)
	if len(hc.Users) == 0 {
		c.RemoveHost(host)
		return ""
	}
	if _, ok := hc.Users[hc.User]; !ok {
		hc.User = hc.Logins()[0]
	}
	c.Hosts[host] = hc
	return hc.User
}
