package sync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/cmd/repo/clone"
)

// git runs git in the current directory and returns its standard output.
// A failure carries git's standard error.
func git(args ...string) (string, error) {
	return gitEnv(nil, args...)
}

// gitEnv runs git like git with env added to the environment.
func gitEnv(env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s failed: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// fetch fetches branch of remote into FETCH_HEAD. git gets the token only
// when the remote's URL, after any insteadOf rule, is https on the
// configured host (see clone.CredentialEnv), and never prompts.
func fetch(opts *SyncOptions, remote, branch string) error {
	remoteURL, err := git("remote", "get-url", "--", remote)
	if err != nil {
		return err
	}
	existing, err := clone.ConfigCount()
	if err != nil {
		return err
	}
	credEnv, err := clone.CredentialEnv(opts.Host, opts.Token, remoteURL, existing)
	if err != nil {
		return err
	}
	env := append([]string{"GIT_TERMINAL_PROMPT=0"}, credEnv...)
	_, err = gitEnv(env, "fetch", "-q", "--", remote, "refs/heads/"+branch)
	return err
}

func hasLocalBranch(branch string) bool {
	_, err := git("rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func isAncestor(ancestor, progeny string) bool {
	_, err := git("merge-base", "--is-ancestor", ancestor, progeny)
	return err == nil
}

// currentBranch returns the checked-out branch, "" on a detached HEAD.
func currentBranch() string {
	ref, _ := git("symbolic-ref", "--quiet", "--short", "HEAD")
	return ref
}

// isDirty reports uncommitted or untracked changes, as gh counts them.
func isDirty() (bool, error) {
	out, err := git("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}
