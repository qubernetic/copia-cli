// Package cmdutil provides shared CLI helpers including the Factory for
// dependency injection, authentication resolution, repository detection,
// JSON output flags, and command grouping utilities.
package cmdutil

import (
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

	// Overrides (flags only ��� env vars resolved in ResolveAuth)
	Token string
	Host  string
	Repo  string // -R/--repo flag override for BaseRepo
}

// ResolveAuth returns the host and token, resolving in order:
// 1. --token/--host flags (highest priority)
// 2. COPIA_TOKEN/COPIA_HOST env vars
// 3. Config file (lowest priority)
func (f *Factory) ResolveAuth() (host, token string, err error) {
	// 1. Flags (already set by Cobra if provided)
	host = f.Host
	token = f.Token

	// 2. Env vars (only if flag not set)
	if host == "" {
		host = os.Getenv("COPIA_HOST")
	}
	if token == "" {
		token = os.Getenv("COPIA_TOKEN")
	}

	// 3. Config fallback
	if host == "" || token == "" {
		cfg, err := f.Config()
		if err != nil {
			return "", "", err
		}
		if host == "" {
			h, _ := cfg.DefaultHost()
			host = h
		}
		if token == "" && host != "" {
			if hc, ok := cfg.Hosts[host]; ok {
				token = hc.Token
			}
		}
	}

	if host == "" {
		return "", "", fmt.Errorf("no host configured. Run 'copia auth login' first")
	}
	if token == "" {
		return "", "", fmt.Errorf("no token configured for %s. Run 'copia auth login' first", host)
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
