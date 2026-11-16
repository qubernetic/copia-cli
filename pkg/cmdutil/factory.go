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
// a host --host or COPIA_HOST names, never to one the config picks.
const envTokenIgnored = "COPIA_TOKEN is ignored because no host is named; set COPIA_HOST (or --host) to the host the token belongs to"

// envTokenOtherHost explains why COPIA_TOKEN was not used: --host names
// another host than COPIA_HOST, the host the token belongs to.
const envTokenOtherHost = "COPIA_TOKEN is ignored because --host names another host than COPIA_HOST; the token is sent only to the host COPIA_HOST names"

// ErrNoToken is wrapped by the error ResolveAuth returns when neither a flag,
// COPIA_TOKEN nor a stored token supplies a token for the host.
var ErrNoToken = errors.New("no token configured")

// ResolveAuth returns the host and token. The host comes from --host, then
// COPIA_HOST, then the config's default host. The token comes from:
// 1. --token (highest priority)
// 2. COPIA_TOKEN, only when the host came from COPIA_HOST, or from --host
// with COPIA_HOST unset or naming the same host, so the token never goes to
// a host the config picks or to another host than COPIA_HOST names
// 3. The stored token of the host's active account: the config file, then
// the OS keyring (lowest priority)
//
// When COPIA_TOKEN is set but passed over, the stored token is used and a
// warning goes to ErrOut once; the token value itself is never printed.
func (f *Factory) ResolveAuth() (host, token string, err error) {
	host = f.Host
	token = f.Token
	envHost := os.Getenv("COPIA_HOST")
	if host == "" {
		host = envHost
	}
	// reason says why COPIA_TOKEN does not apply to host, empty when it does.
	reason := ""
	switch {
	case host == "":
		reason = envTokenIgnored
	case envHost != "" && !sameHost(host, envHost):
		reason = envTokenOtherHost
	}
	envToken := os.Getenv("COPIA_TOKEN")
	if token == "" && reason == "" {
		token = envToken
	}
	// COPIA_TOKEN was passed over for the stored token of the host.
	envTokenSkipped := token == "" && envToken != ""
	hint := ""
	if envTokenSkipped {
		hint = ". " + reason
	}

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
				return "", "", fmt.Errorf("reading the token for %s: %w%s", host, err, hint)
			}
		}
	}

	if host == "" {
		return "", "", fmt.Errorf("no host configured. Run 'copia auth login' first%s", hint)
	}
	if token == "" {
		return "", "", fmt.Errorf("%w for %s. Run 'copia auth login' first%s", ErrNoToken, host, hint)
	}
	if envTokenSkipped && !f.envTokenWarned {
		f.envTokenWarned = true
		if f.IOStreams != nil {
			_, _ = fmt.Fprintln(f.IOStreams.ErrOut, "warning: "+reason)
		}
	}
	return host, token, nil
}

// sameHost reports whether a and b name the same host: the host name
// compared ASCII case-insensitively and the same port, a missing port being
// 443. A host that does not parse or has a non-ASCII name is never the same
// host.
func sameHost(a, b string) bool {
	u, err := ParseHost(a)
	if err != nil {
		return false
	}
	if _, err := ParseHost(b); err != nil {
		return false
	}
	return MatchesHost(u, "https://"+b)
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
