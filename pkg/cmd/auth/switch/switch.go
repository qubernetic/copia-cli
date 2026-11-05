// Package authswitch implements `copia-cli auth switch`. The package is not
// named after its directory because switch is a Go keyword, as in gh.
package authswitch

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// SwitchOptions holds all inputs for the switch command.
type SwitchOptions struct {
	IO         *iostreams.IOStreams
	Host       string // empty considers every host
	User       string // empty considers every account
	ConfigPath string
}

// NewCmdSwitch creates the `copia auth switch` command.
func NewCmdSwitch(f *cmdutil.Factory) *cobra.Command {
	opts := &SwitchOptions{}

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Switch the active account",
		Long:  "Switch the active account of a Copia host and make that host the default host. With --host and --user it switches to that account. With --host alone it switches to the host's account when the host has one, and to the other account when it has two. Without flags it switches to the other account when exactly two accounts exist in total. In every other case it fails and lists the accounts to choose from. The token of each account stays where it is stored; COPIA_TOKEN still takes precedence over the token of the active account, and while it is set without COPIA_HOST the command refuses to switch to an account on another host.",
		Example: "  $ copia-cli auth switch\n  $ copia-cli auth switch --host app.copia.io --user john",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			if opts.Host == "" {
				opts.Host = f.Host
			}
			return switchRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Host, "host", "", "The Copia host of the account to switch to")
	cmd.Flags().StringVarP(&opts.User, "user", "u", "", "The account to switch to")

	return cmd
}

type account struct{ host, login string }

func switchRun(opts *SwitchOptions) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	var candidates []account
	for _, host := range cfg.HostNames() {
		if opts.Host != "" && host != opts.Host {
			continue
		}
		for _, login := range cfg.Hosts[host].Logins() {
			if opts.User == "" || login == opts.User {
				candidates = append(candidates, account{host, login})
			}
		}
	}
	if len(candidates) == 0 {
		switch {
		case opts.Host != "" && (cfg.Hosts[opts.Host] == nil || opts.User == ""):
			// An unknown host, or a host entry without any account.
			return fmt.Errorf("not logged in to %s", opts.Host)
		case opts.Host != "":
			return fmt.Errorf("not logged in to %s account %s", opts.Host, opts.User)
		case opts.User != "":
			return fmt.Errorf("not logged in to any host as %s", opts.User)
		default:
			return fmt.Errorf("not logged in to any hosts")
		}
	}

	target, ok := pick(cfg, candidates)
	if !ok {
		lines := make([]string, len(candidates))
		for i, c := range candidates {
			lines[i] = c.host + " " + c.login
		}
		return fmt.Errorf("unable to determine which account to switch to, specify --host and --user; accounts:\n%s", strings.Join(lines, "\n"))
	}

	// Without COPIA_HOST, COPIA_TOKEN is sent to the default host, so moving
	// default_host would send it to a host it was not issued for.
	if defaultHost, _ := cfg.DefaultHost(); os.Getenv("COPIA_TOKEN") != "" && os.Getenv("COPIA_HOST") == "" && target.host != defaultHost {
		return fmt.Errorf("COPIA_TOKEN is set and is sent to the default host %s; unset it before switching to an account on %s", defaultHost, target.host)
	}

	if err := cfg.SwitchAccount(target.host, target.login); err != nil {
		return err
	}
	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Switched active account for %s to %s\n", target.host, target.login)
	if os.Getenv("COPIA_TOKEN") != "" {
		_, _ = fmt.Fprintln(opts.IO.ErrOut, "warning: COPIA_TOKEN is set and is used instead of the token of the active account")
	}
	return nil
}

// pick chooses the account to switch to: the only candidate, or of two the
// one that is not in use. The account in use is the active account of the
// default host; failing that, the candidate that is its host's active account
// while the other is not. Any other case is ambiguous.
func pick(cfg *config.Config, candidates []account) (account, bool) {
	switch len(candidates) {
	case 1:
		return candidates[0], true
	case 2:
	default:
		return account{}, false
	}
	defaultHost, _ := cfg.DefaultHost()
	active := func(c account) bool { return cfg.Hosts[c.host].User == c.login }
	for i, c := range candidates {
		if c.host == defaultHost && active(c) {
			return candidates[1-i], true
		}
	}
	for i, c := range candidates {
		if active(c) && !active(candidates[1-i]) {
			return candidates[1-i], true
		}
	}
	return account{}, false
}
