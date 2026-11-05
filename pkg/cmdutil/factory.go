// Package cmdutil provides shared CLI helpers including the Factory for
// dependency injection, authentication resolution, repository detection,
// JSON output flags, and command grouping utilities.
package cmdutil

import (
	"errors"
	"fmt"
	"os"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// Factory provides shared dependencies to all commands.
type Factory struct {
	IOStreams *iostreams.IOStreams
	Config   func() (*config.Config, error)
	BaseRepo func() (string, string, error) // returns owner, repo
	Browser  Browser
	// Keyring is the OS keyring the stored tokens live in; nil reads the
	// config file only.
	Keyring config.TokenStore

	// Overrides (flags only ��� env vars resolved in ResolveAuth)
	Token string
	Host  string
	Repo  string // -R/--repo flag override for BaseRepo

	// envTokenWarned is set once the warning that COPIA_TOKEN is ignored
	// has been printed, so it appears at most once per process.
	envTokenWarned bool
}

// envTokenIgnored explains why COPIA_TOKEN was not used: it is sent only to
// the host --host or COPIA_HOST names.
const envTokenIgnored = "COPIA_TOKEN is ignored because no host is named; set COPIA_HOST (or --host) to the host the token belongs to"

// ResolveAuth returns the host and token. The host comes from --host, then
// COPIA_HOST, then the config's default host. The token comes from:
// 1. --token (highest priority)
// 2. COPIA_TOKEN, only when the host came from --host or COPIA_HOST, so the
// token never goes to a host the config picks
// 3. The stored token of the host's active account: the config file, then
// the OS keyring (lowest priority)
//
// When COPIA_TOKEN is set but the host came from the config, the stored
// token is used and a warning goes to ErrOut once; the token value itself
// is never printed.
func (f *Factory) ResolveAuth() (host, token string, err error) {
	host = f.Host
	token = f.Token
	if host == "" {
		host = os.Getenv("COPIA_HOST")
	}
	hostNamed := host != ""
	envToken := os.Getenv("COPIA_TOKEN")
	if token == "" && hostNamed {
		token = envToken
	}
	// COPIA_TOKEN was passed over for the token of a host the config picks.
	envTokenSkipped := token == "" && envToken != ""

	if host == "" || token == "" {
		cfg, err := f.Config()
		if err != nil {
			return "", "", err
		}
		if host == "" {
			host, _ = cfg.DefaultHost()
		}
		if token == "" && host != "" {
			token, _, err = config.LookupToken(cfg, host, f.Keyring)
			if err != nil && !errors.Is(err, config.ErrTokenNotFound) {
				return "", "", fmt.Errorf("reading the token for %s: %w", host, err)
			}
		}
	}

	hint := ""
	if envTokenSkipped {
		hint = ". " + envTokenIgnored
	}
	if host == "" {
		return "", "", fmt.Errorf("no host configured. Run 'copia auth login' first%s", hint)
	}
	if token == "" {
		return "", "", fmt.Errorf("no token configured for %s. Run 'copia auth login' first%s", host, hint)
	}
	if envTokenSkipped && !f.envTokenWarned {
		f.envTokenWarned = true
		if f.IOStreams != nil {
			_, _ = fmt.Fprintln(f.IOStreams.ErrOut, "warning: "+envTokenIgnored)
		}
	}
	return host, token, nil
}

// ResolveHost returns the host in the order of ResolveAuth (--host flag,
// COPIA_HOST, the config's default host) without requiring a token, for
// commands that make no API call.
func (f *Factory) ResolveHost() (string, error) {
	host := f.Host
	if host == "" {
		host = os.Getenv("COPIA_HOST")
	}
	if host == "" {
		cfg, err := f.Config()
		if err != nil {
			return "", err
		}
		host, _ = cfg.DefaultHost()
	}
	if host == "" {
		return "", fmt.Errorf("no host configured. Run 'copia auth login' first")
	}
	return host, nil
}

// ResolveRepo returns owner and repo, resolving in order:
// 1. -R/--repo flag (highest priority)
// 2. BaseRepo from git remote (fallback)
func (f *Factory) ResolveRepo() (owner, repo string, err error) {
	if f.Repo != "" {
		return SplitOwnerRepo(f.Repo)
	}
	if f.BaseRepo == nil {
		return "", "", fmt.Errorf("could not determine repository. Use -R owner/repo or run from inside a git repository")
	}
	return f.BaseRepo()
}

// ValidateLimit returns an error if limit is not a positive integer.
func ValidateLimit(limit int) error {
	if limit < 1 {
		return fmt.Errorf("invalid limit: %d", limit)
	}
	return nil
}

// ConfigValue returns the value of a setting key of the config file, or its
// default when it is unset. An unknown key or a config file that cannot be
// read gives the empty string, so the caller falls back to the next source.
func (f *Factory) ConfigValue(key string) string {
	if f.Config == nil {
		return ""
	}
	cfg, err := f.Config()
	if err != nil {
		return ""
	}
	value, err := cfg.Get(key)
	if err != nil {
		return ""
	}
	return value
}

// OutputFormat returns the default output format of the commands that take
// --json, resolving in order: COPIA_OUTPUT, the config key output, then
// table. A value other than table or json is an error.
func (f *Factory) OutputFormat() (string, error) {
	value, source := os.Getenv("COPIA_OUTPUT"), "COPIA_OUTPUT"
	if value == "" {
		value, source = f.ConfigValue("output"), "config key output"
	}
	switch value {
	case "":
		return OutputTable, nil
	case OutputTable, OutputJSON:
		return value, nil
	}
	return "", fmt.Errorf("invalid %s %q: valid values are %s, %s", source, value, OutputTable, OutputJSON)
}

// Pager returns the pager command, resolving in order: COPIA_PAGER, the
// config key pager, then PAGER. COPIA_PAGER set to the empty string turns
// the pager off.
func (f *Factory) Pager() string {
	if value, ok := os.LookupEnv("COPIA_PAGER"); ok {
		return value
	}
	if value := f.ConfigValue("pager"); value != "" {
		return value
	}
	return os.Getenv("PAGER")
}
