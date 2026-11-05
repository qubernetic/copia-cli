package clone

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CloneOptions holds all inputs for the repo clone command.
type CloneOptions struct {
	IO    *iostreams.IOStreams
	Host  string
	Token string
	Repo  string
	Dir   string
}

// NewCmdClone creates the `copia repo clone` command.
func NewCmdClone(f *cmdutil.Factory) *cobra.Command {
	opts := &CloneOptions{}

	cmd := &cobra.Command{
		Use:   "clone <owner/repo | URL> [<directory>]",
		Short: "Clone a repository",
		Long: `Clone a Copia repository locally. The repository can be specified as
owner/repo or as a full URL. Private repositories are automatically
authenticated using your configured token.

The token is sent only to the configured host: an https URL whose host and
port match it. Any other URL (another host, http, SSH) is cloned without
credentials, and so is any other host the configured one redirects to. git
gets the token from a credential helper scoped to the configured host,
never from the clone URL or the command line, so it is not stored in the
remote or in your credential store. A token with a character outside
printable ASCII, such as a line break, is refused before git runs.`,
		Example: `  # Clone by owner/repo
  $ copia-cli repo clone my-org/my-repo

  # Clone by URL
  $ copia-cli repo clone https://app.copia.io/my-org/my-repo.git`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Repo = args[0]

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			if len(args) > 1 {
				opts.Dir = args[1]
			}

			return CloneRun(opts)
		},
	}

	return cmd
}

func CloneRun(opts *CloneOptions) error {
	existing, err := configCount(os.Getenv("GIT_CONFIG_COUNT"))
	if err != nil {
		return err
	}
	args, env, err := cloneCommand(opts, existing)
	if err != nil {
		return err
	}

	gitCmd := exec.Command("git", args...)
	gitCmd.Stdout = opts.IO.Out
	gitCmd.Stderr = opts.IO.ErrOut
	gitCmd.Stdin = os.Stdin
	gitCmd.Env = append(os.Environ(), env...)

	return gitCmd.Run()
}

// credentialHelper answers git's `get` with the token from
// COPIA_CLONE_TOKEN and ignores `store` and `erase`, so git keeps no copy
// of it. The token is read from the environment, never the command line.
const credentialHelper = `!f() { test "$1" = get || return 0; printf 'username=token\npassword=%s\n' "$COPIA_CLONE_TOKEN"; }; f`

// cloneCommand returns the git arguments and the environment entries to add
// for a clone. The token never goes into the arguments: when the URL points
// at the configured host over https, git gets it from credentialHelper,
// which git's config scopes to that host. The first, empty helper entry
// drops the user's helpers for that host only. git asks for the credential
// per origin, so after a redirect to another origin it has none to send.
// existing is the number of GIT_CONFIG_* entries already in the
// environment, which are kept.
func cloneCommand(opts *CloneOptions, existing int) (args, env []string, err error) {
	configured, err := cmdutil.ParseHost(opts.Host)
	if err != nil {
		return nil, nil, err
	}
	if !cmdutil.IsPrintableASCII(opts.Token) {
		return nil, nil, fmt.Errorf("invalid token: it may contain only printable ASCII characters")
	}
	cloneURL := buildCloneURL(opts.Host, opts.Repo)

	args = []string{"clone", "--", cloneURL}
	if opts.Dir != "" {
		args = append(args, opts.Dir)
	}

	env = []string{"GIT_TERMINAL_PROMPT=0"}
	if opts.Token == "" || !cmdutil.MatchesHost(configured, cloneURL) {
		return args, env, nil
	}

	key := credentialKey(configured)
	env = append(env,
		"COPIA_CLONE_TOKEN="+opts.Token,
		fmt.Sprintf("GIT_CONFIG_COUNT=%d", existing+2),
		fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", existing, key),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=", existing),
		fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", existing+1, key),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", existing+1, credentialHelper),
	)
	return args, env, nil
}

// credentialKey returns the git config key that scopes a credential helper
// to the configured host: its lower-case name, IPv6 in brackets, and the
// port unless it is 443.
func credentialKey(configured *url.URL) string {
	host := strings.ToLower(configured.Hostname())
	if port := configured.Port(); port != "" && port != "443" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return "credential.https://" + host + ".helper"
}

// configCount parses GIT_CONFIG_COUNT. Unset or empty means no entries; any
// other value that is not a non-negative integer is an error, as it is for
// git, so the entries already set are never overwritten.
func configCount(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid GIT_CONFIG_COUNT %q: must be a non-negative integer", value)
	}
	return n, nil
}

func buildCloneURL(host, repo string) string {
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	return fmt.Sprintf("https://%s/%s.git", host, repo)
}
