package shared

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// PRRef is a pull request named on the command line. Host, Owner and Repo
// are set only when the argument was a pull request URL.
type PRRef struct {
	Host   string
	Owner  string
	Repo   string
	Number int64
}

// ParsePRArg accepts a pull request number (`7` or `#7`) or a pull request
// URL (`https://<host>/<owner>/<repo>/pulls/7`), as gh does.
func ParsePRArg(arg string) (PRRef, error) {
	invalid := fmt.Errorf("invalid pull request number or URL: %s", arg)

	if !strings.HasPrefix(arg, "https://") {
		n, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64)
		if err != nil || n < 1 {
			return PRRef{}, invalid
		}
		return PRRef{Number: n}, nil
	}

	u, err := url.Parse(arg)
	if err != nil {
		return PRRef{}, invalid
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pulls" {
		return PRRef{}, invalid
	}
	n, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || n < 1 {
		return PRRef{}, invalid
	}
	return PRRef{Host: u.Host, Owner: parts[0], Repo: parts[1], Number: n}, nil
}

// Resolve returns the repository the pull request lives in: the one from the
// URL, or owner and repo for a bare number. A URL on another host than the
// active one fails, so the token is never sent to a host it was not made for.
func (r PRRef) Resolve(host, owner, repo string) (string, string, error) {
	if r.Host == "" {
		return owner, repo, nil
	}
	if !strings.EqualFold(r.Host, host) {
		return "", "", fmt.Errorf("pull request URL host %s does not match the active host %s", r.Host, host)
	}
	return r.Owner, r.Repo, nil
}
