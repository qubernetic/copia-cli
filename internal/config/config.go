// Package config manages the CLI configuration file (~/.config/copia/config.yml),
// including host credentials, default host resolution, and file permissions.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// HostConfig stores credentials for a single Copia/Gitea instance.
type HostConfig struct {
	Token string `yaml:"token"`
	User  string `yaml:"user"`
}

// Config is the top-level configuration: the hosts and the settings that
// `copia-cli config` reads and writes.
type Config struct {
	Hosts   map[string]*HostConfig `yaml:"hosts"`
	Output  string                 `yaml:"output,omitempty"`
	Browser string                 `yaml:"browser,omitempty"`
	Pager   string                 `yaml:"pager,omitempty"`
	Editor  string                 `yaml:"editor,omitempty"`
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

// Save writes config to path with 0600 permissions.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// WriteFile keeps the mode of an existing file, so drop every bit beyond
	// 0600 before the token is written; a read-only file stays read-only.
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&^0600 != 0 {
		if err := os.Chmod(path, info.Mode().Perm()&0600); err != nil {
			return fmt.Errorf("securing config: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// DefaultHost returns the first host entry. Returns empty string and nil if no hosts configured.
func (c *Config) DefaultHost() (string, *HostConfig) {
	for host, hc := range c.Hosts {
		return host, hc
	}
	return "", nil
}
