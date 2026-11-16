// Package setupgit implements `copia-cli auth setup-git`, which makes
// copia-cli the git credential helper of the logged-in Copia hosts, as gh's
// `auth setup-git` does.
package setupgit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// SetupGitOptions holds all inputs for the setup-git command.
type SetupGitOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Host   string // empty configures every logged-in host
	Force  bool   // configure Host even when it is not logged in
	// Executable is the path of copia-cli that git runs as the helper.
	Executable string
}

// NewCmdSetupGit creates the `copia auth setup-git` command.
func NewCmdSetupGit(f *cmdutil.Factory) *cobra.Command {
	opts := &SetupGitOptions{}

	cmd := &cobra.Command{
		Use:   "setup-git",
		Short: "Set up git to use copia-cli as a credential helper",
		Long: `Configure git to use copia-cli as the credential helper of every Copia host you are logged in to, so git clone, fetch and push over https authenticate with the token of the host's active account.

For each host it writes two values of credential.https://<host>.helper to the global git config: an empty value, which drops every helper configured before it for that host, and then "!'<path of copia-cli>' auth git-credential". A helper configured after it, later in the global config or in a repository's config, still takes part. Running it again changes nothing.

With --host it configures that host only, which must be logged in unless --force is given.`,
		Example: "  $ copia-cli auth setup-git\n  $ copia-cli auth setup-git --host app.copia.io",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Config = f.Config
			if opts.Host == "" {
				opts.Host = f.Host
			}
			if opts.Force && opts.Host == "" {
				return fmt.Errorf("--force must be used with --host")
			}
			exe, err := executable()
			if err != nil {
				return fmt.Errorf("finding the path of copia-cli: %w", err)
			}
			opts.Executable = exe
			return setupGitRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Host, "host", "", "The Copia host to configure git for")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Configure the host even when you are not logged in to it; needs --host")

	return cmd
}

// executable returns the path git runs as the helper: the copia-cli found in
// PATH when it is a symlink to the running binary, so a symlink such as
// Homebrew's keeps working after an upgrade, as in gh; otherwise the running
// binary. A hard link is another file name, so it does not count.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	inPath, err := exec.LookPath("copia-cli")
	if err != nil {
		return exe, nil
	}
	inPath, err = filepath.Abs(inPath)
	if err != nil {
		return exe, nil
	}
	info, err := os.Lstat(inPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return exe, nil
	}
	target, errA := filepath.EvalSymlinks(inPath)
	realExe, errB := filepath.EvalSymlinks(exe)
	if errA == nil && errB == nil && target == realExe {
		return inPath, nil
	}
	return exe, nil
}

func setupGitRun(opts *SetupGitOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	var loggedIn []string
	for _, host := range cfg.HostNames() {
		if len(cfg.Hosts[host].Logins()) > 0 {
			loggedIn = append(loggedIn, host)
		}
	}

	hosts := loggedIn
	if opts.Host != "" {
		if !opts.Force && !slices.Contains(loggedIn, opts.Host) {
			return fmt.Errorf("not logged in to %s. Run 'copia-cli auth login --host %s' first, or use --force", opts.Host, opts.Host)
		}
		hosts = []string{opts.Host}
	}
	if len(hosts) == 0 {
		return fmt.Errorf("not logged in to any hosts. Run 'copia-cli auth login' first")
	}

	// Every host goes into a git config key, so check them all before the
	// first change.
	for _, host := range hosts {
		if _, err := cmdutil.ParseHost(host); err != nil {
			return err
		}
	}
	for _, host := range hosts {
		if err := configure(host, opts.Executable); err != nil {
			return err
		}
	}
	return nil
}

// configure sets copia-cli as the credential helper of the https URL of host
// in the global git config. The empty value first drops every helper
// configured before it; one configured after it still takes part, as in gh.
func configure(host, executable string) error {
	key := "credential.https://" + host + ".helper"
	helper := "!" + shellQuote(executable) + " auth git-credential"
	for _, args := range [][]string{
		{"config", "--global", "--replace-all", key, ""},
		{"config", "--global", "--add", key, helper},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set up the git credential helper for %s: %w: %s", host, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// shellQuote quotes s for the shell git runs a helper starting with ! in.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
