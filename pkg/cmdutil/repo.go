package cmdutil

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// SplitOwnerRepo parses "owner/repo" into separate components.
func SplitOwnerRepo(nwo string) (string, string, error) {
	i := strings.IndexByte(nwo, '/')
	if i > 0 && i < len(nwo)-1 {
		return nwo[:i], nwo[i+1:], nil
	}
	return "", "", fmt.Errorf("expected owner/repo format, got %q", nwo)
}

// ParseRemoteURL extracts owner and repo from a git remote URL.
// Supports HTTPS (https://host/owner/repo.git) and SSH (git@host:owner/repo.git).
func ParseRemoteURL(rawURL string) (owner, repo string, err error) {
	// SSH format: git@host:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("could not parse SSH remote URL: %s", rawURL)
		}
		path := strings.TrimSuffix(parts[1], ".git")
		return SplitOwnerRepo(path)
	}

	// HTTPS format: https://host/owner/repo.git
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("could not parse remote URL: %s", rawURL)
	}

	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	return SplitOwnerRepo(path)
}

// RemoteResolvedKey is the git config key under remote.<name> that marks the
// default remote of a repository, set by `repo set-default` (gh uses
// gh-resolved).
const RemoteResolvedKey = "copia-resolved"

// Remote is a git remote whose URL names a repository.
type Remote struct {
	Name     string
	Owner    string
	Repo     string
	Resolved bool // marked as the default by `repo set-default`
}

// FullName returns "owner/repo".
func (r Remote) FullName() string {
	return r.Owner + "/" + r.Repo
}

// ListRemotes returns the git remotes of the current repository in config
// order. A remote whose URL does not name an owner and a repository is left
// out.
func ListRemotes() ([]Remote, error) {
	if err := exec.Command("git", "rev-parse", "--git-dir").Run(); err != nil {
		return nil, fmt.Errorf("must be run from inside a git repository")
	}
	// git remote -v applies url.<base>.insteadOf, as git remote get-url does.
	out, err := exec.Command("git", "remote", "-v").Output()
	if err != nil {
		return nil, fmt.Errorf("could not list the git remotes: %w", err)
	}
	var names []string
	urls := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		// A partial clone appends its filter: "origin URL (fetch) [blob:none]".
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "(fetch)" {
			continue
		}
		if _, seen := urls[fields[0]]; !seen {
			names = append(names, fields[0])
			urls[fields[0]] = fields[1]
		}
	}

	resolved := map[string]bool{}
	for key, value := range resolvedKeys() {
		name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), "."+RemoteResolvedKey)
		resolved[name] = value != ""
	}

	var remotes []Remote
	for _, name := range names {
		owner, repo, err := ParseRemoteURL(urls[name])
		if err != nil {
			continue
		}
		remotes = append(remotes, Remote{Name: name, Owner: owner, Repo: repo, Resolved: resolved[name]})
	}
	return remotes, nil
}

// resolvedKeys returns the remote.<name>.copia-resolved keys of the current
// repository with their values, including those of remotes ListRemotes leaves
// out.
func resolvedKeys() map[string]string {
	// Exit status 1 means no remote is marked.
	out, _ := exec.Command("git", "config", "--get-regexp", `^remote\..*\.`+RemoteResolvedKey+`$`).Output()
	keys := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		keys[key] = value
	}
	return keys
}

// SetDefaultRemote marks the remote name as the default of the current
// repository and removes the mark from every other remote.
func SetDefaultRemote(name string) error {
	if err := UnsetDefaultRemote(); err != nil {
		return err
	}
	out, err := exec.Command("git", "config", "remote."+name+"."+RemoteResolvedKey, "base").CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not set the default remote: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// UnsetDefaultRemote removes the default mark from every remote of the
// current repository, also from a remote whose URL names no repository.
func UnsetDefaultRemote() error {
	if err := exec.Command("git", "rev-parse", "--git-dir").Run(); err != nil {
		return fmt.Errorf("must be run from inside a git repository")
	}
	for key := range resolvedKeys() {
		out, err := exec.Command("git", "config", "--unset-all", key).CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not unset the default remote: %s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// DetectBaseRepo returns a function that detects owner/repo from the git
// remote marked by `repo set-default`, otherwise from the remote "origin".
func DetectBaseRepo() func() (string, string, error) {
	return func() (string, string, error) {
		remotes, err := ListRemotes()
		if err != nil {
			return "", "", fmt.Errorf("not in a git repository or no origin remote configured")
		}
		for _, r := range remotes {
			if r.Resolved {
				return r.Owner, r.Repo, nil
			}
		}
		for _, r := range remotes {
			if r.Name == "origin" {
				return r.Owner, r.Repo, nil
			}
		}
		return "", "", fmt.Errorf("no default remote and no origin remote configured; run `copia-cli repo set-default` or use -R owner/repo")
	}
}
