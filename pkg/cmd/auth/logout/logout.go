package logout

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// LogoutOptions holds all inputs for the logout command.
type LogoutOptions struct {
	IO         *iostreams.IOStreams
	Host       string
	User       string // the account to log out of; empty is the active one
	ConfigPath string
	// Keyring is the OS keyring the token is deleted from as well; nil
	// touches the config file only.
	Keyring config.TokenStore
}

// NewCmdLogout creates the `copia auth logout` command.
func NewCmdLogout(f *cmdutil.Factory) *cobra.Command {
	opts := &LogoutOptions{}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of a Copia instance",
		Long:  "Remove the stored authentication credentials of one account on a Copia host from the config file and the OS keyring. This does not revoke the token on the server. Without --host it logs out of the default host, without --user of the host's active account. When the active account is logged out, the alphabetically first remaining account of the host becomes active. When the host's last account is logged out, the host is removed, and when it was the default host, the alphabetically first remaining host becomes the default.",
		Example: "  $ copia-cli auth logout --host app.copia.io\n  $ copia-cli auth logout --host app.copia.io --user john",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.Keyring = f.Keyring
			if f.Host != "" {
				opts.Host = f.Host
			}
			if opts.Host == "" {
				cfg, err := config.Load(opts.ConfigPath)
				if err != nil {
					return err
				}
				h, _ := cfg.DefaultHost()
				opts.Host = h
			}
			if opts.Host == "" {
				return fmt.Errorf("no host specified and no default configured")
			}
			return logoutRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Host, "host", "", "Copia instance hostname")
	cmd.Flags().StringVarP(&opts.User, "user", "u", "", "The account to log out of")

	return cmd
}

func logoutRun(opts *LogoutOptions) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	hc, ok := cfg.Hosts[opts.Host]
	if !ok {
		return fmt.Errorf("not logged in to %s", opts.Host)
	}
	user := opts.User
	if user == "" {
		user = hc.User
	}
	if user == "" && len(hc.Logins()) == 0 {
		// A host entry without any account holds no keyring entry.
		cfg.RemoveHost(opts.Host)
		if err := config.Save(opts.ConfigPath, cfg); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(opts.IO.Out, "Logged out of %s\n", opts.Host)
		return nil
	}
	if !slices.Contains(hc.Logins(), user) {
		return fmt.Errorf("not logged in to %s account %s", opts.Host, user)
	}

	// A keyring that fails does not keep the account in the config file:
	// the warning tells the user to check the keyring by hand.
	if opts.Keyring != nil {
		err := opts.Keyring.Delete(opts.Host, user)
		if err != nil && !errors.Is(err, config.ErrTokenNotFound) {
			_, _ = fmt.Fprintf(opts.IO.ErrOut, "warning: could not delete the token of %s on %s from the OS keyring: %v\n", user, opts.Host, err)
		}
	}

	wasActive := user == hc.User
	active := cfg.RemoveAccount(opts.Host, user)

	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Logged out of %s account %s\n", opts.Host, user)
	if wasActive && active != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "Switched active account for %s to %s\n", opts.Host, active)
	}
	return nil
}
