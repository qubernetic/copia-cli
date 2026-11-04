package shared

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// IssueRef is an issue named on the command line. Host, Owner and Repo are
// set only when the argument was an issue URL.
type IssueRef struct {
	Host   string
	Owner  string
	Repo   string
	Number int64
}

// ParseIssueArg accepts an issue number (`12` or `#12`) or an issue URL
// (`https://<host>/<owner>/<repo>/issues/12`), as gh does.
func ParseIssueArg(arg string) (IssueRef, error) {
	invalid := fmt.Errorf("invalid issue number or URL: %s", arg)

	if !strings.HasPrefix(arg, "https://") {
		n, err := strconv.ParseInt(strings.TrimPrefix(arg, "#"), 10, 64)
		if err != nil || n < 1 {
			return IssueRef{}, invalid
		}
		return IssueRef{Number: n}, nil
	}

	u, err := url.Parse(arg)
	if err != nil {
		return IssueRef{}, invalid
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "issues" {
		return IssueRef{}, invalid
	}
	n, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || n < 1 {
		return IssueRef{}, invalid
	}
	return IssueRef{Host: u.Host, Owner: parts[0], Repo: parts[1], Number: n}, nil
}

// Resolve returns the repository the issue lives in: the one from the URL,
// or owner and repo for a bare number. A URL on another host than the active
// one fails, so the token is never sent to a host it was not made for.
func (r IssueRef) Resolve(host, owner, repo string) (string, string, error) {
	if r.Host == "" {
		return owner, repo, nil
	}
	if !strings.EqualFold(r.Host, host) {
		return "", "", fmt.Errorf("issue URL host %s does not match the active host %s", r.Host, host)
	}
	return r.Owner, r.Repo, nil
}
