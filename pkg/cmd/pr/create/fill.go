package create

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type commit struct {
	Subject string
	Body    string
}

// fill sets the head branch, title and body from the local git history, the
// way `gh pr create --fill` does. Values the user gave are kept.
func fill(opts *CreateOptions) error {
	if opts.Head == "" {
		head, err := gitOutput("symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return fmt.Errorf("could not determine the current branch: %w", err)
		}
		opts.Head = head
	}

	commits, err := commitsBetween(baseRef(opts.Base), opts.Head)
	if err != nil {
		return fmt.Errorf("could not read commits between %s and %s: %w", opts.Base, opts.Head, err)
	}

	var title, body string
	switch len(commits) {
	case 0:
		return nil
	case 1:
		title, body = commits[0].Subject, commits[0].Body
	default:
		title = opts.Head
		subjects := make([]string, len(commits))
		for i, c := range commits {
			subjects[i] = "- " + c.Subject
		}
		body = strings.Join(subjects, "\n")
	}

	if opts.Title == "" {
		opts.Title = title
	}
	if opts.Body == "" {
		opts.Body = body
	}
	return nil
}

// baseRef prefers the remote-tracking branch of base, which is what the pull
// request is compared against, and falls back to the local branch.
func baseRef(base string) string {
	remote := "origin/" + base
	if _, err := gitOutput("rev-parse", "--verify", "--quiet", "refs/remotes/"+remote); err == nil {
		return remote
	}
	return base
}

// commitsBetween lists the commits on head that are not on base, oldest first.
func commitsBetween(base, head string) ([]commit, error) {
	out, err := gitOutput("-c", "log.ShowSignature=false", "log", "--cherry", "--reverse",
		"--pretty=format:%s%x00%b%x1e", base+"..."+head)
	if err != nil {
		return nil, err
	}

	var commits []commit
	for _, entry := range strings.Split(out, "\x1e") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		subject, body, _ := strings.Cut(entry, "\x00")
		commits = append(commits, commit{Subject: subject, Body: strings.TrimSpace(body)})
	}
	return commits, nil
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
