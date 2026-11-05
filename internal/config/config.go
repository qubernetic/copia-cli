// Package config manages the CLI configuration file (~/.config/copia/config.yml),
// including host credentials, default host resolution, and file permissions.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// HostConfig stores credentials for a single Copia/Gitea instance.
type HostConfig struct {
	Token string `yaml:"token"`
	User  string `yaml:"user"`
	// Extra keeps the keys this version does not know, so a save writes
	// them back unchanged.
	Extra map[string]any `yaml:",inline"`
}

// Config is the top-level configuration: the hosts and the settings that
// `copia-cli config` reads and writes.
type Config struct {
	Hosts   map[string]*HostConfig `yaml:"hosts"`
	Output  string                 `yaml:"output,omitempty"`
	Browser string                 `yaml:"browser,omitempty"`
	Pager   string                 `yaml:"pager,omitempty"`
	Editor  string                 `yaml:"editor,omitempty"`
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
	return cfg, nil
}

// Save writes config to path as a new file with 0600 permissions. It refuses
// a path that is not a regular file (a symlink, for example) and an existing
// file the user cannot write, and tightens a config directory that group or
// others can write to 0700. The new content goes to a temporary file in the
// same directory that replaces path only once it is complete, so a failed
// save leaves the old file as it was.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
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

// DefaultHost returns the first host entry. Returns empty string and nil if no hosts configured.
func (c *Config) DefaultHost() (string, *HostConfig) {
	for host, hc := range c.Hosts {
		return host, hc
	}
	return "", nil
}
