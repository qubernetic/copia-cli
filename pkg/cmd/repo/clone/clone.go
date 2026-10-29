package clone

import (
	"encoding/base64"
	"fmt"
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
credentials. The token is passed to git as a host-scoped HTTP header, never
in the clone URL, so it is not stored in the remote.`,
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
	args, env := cloneCommand(opts, configCount(os.Getenv("GIT_CONFIG_COUNT")))

	gitCmd := exec.Command("git", args...)
	gitCmd.Stdout = opts.IO.Out
	gitCmd.Stderr = opts.IO.ErrOut
	gitCmd.Stdin = os.Stdin
	gitCmd.Env = append(os.Environ(), env...)

	return gitCmd.Run()
}

// cloneCommand returns the git arguments and the environment entries to add
// for a clone. The token never goes into the arguments: when the URL points
// at the configured host over https, it travels as an Authorization header
// that git's config scopes to that host. existing is the number of
// GIT_CONFIG_* entries already in the environment, which are kept.
func cloneCommand(opts *CloneOptions, existing int) (args, env []string) {
	cloneURL := buildCloneURL(opts.Host, opts.Repo)

	args = []string{"clone", "--", cloneURL}
	if opts.Dir != "" {
		args = append(args, opts.Dir)
	}

	env = []string{"GIT_TERMINAL_PROMPT=0"}
	if opts.Token == "" || !isConfiguredHost(opts.Host, cloneURL) {
		return args, env
	}

	credential := base64.StdEncoding.EncodeToString([]byte("token:" + opts.Token))
	env = append(env,
		fmt.Sprintf("GIT_CONFIG_COUNT=%d", existing+1),
		fmt.Sprintf("GIT_CONFIG_KEY_%d=http.https://%s/.extraHeader", existing, strings.ToLower(opts.Host)),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=Authorization: Basic %s", existing, credential),
	)
	return args, env
}

// configCount parses GIT_CONFIG_COUNT; anything but a non-negative number
// counts as no entries.
func configCount(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// isConfiguredHost reports whether rawURL is an https URL whose host and
// port equal the configured host (case-insensitive, a missing port is 443).
func isConfiguredHost(host, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	configured, err := url.Parse("https://" + host)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Hostname(), configured.Hostname()) && httpsPort(u) == httpsPort(configured)
}

func httpsPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return "443"
}

func buildCloneURL(host, repo string) string {
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	return fmt.Sprintf("https://%s/%s.git", host, repo)
}
