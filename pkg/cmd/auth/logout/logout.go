package logout

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// LogoutOptions holds all inputs for the logout command.
type LogoutOptions struct {
	IO         *iostreams.IOStreams
	Host       string
	ConfigPath string
}

// NewCmdLogout creates the `copia auth logout` command.
func NewCmdLogout(f *cmdutil.Factory) *cobra.Command {
	opts := &LogoutOptions{}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of a Copia instance",
		Long:  "Remove the stored authentication credentials for a Copia host. This does not revoke the token on the server. Without --host it logs out of the default host. When the default host is logged out, the alphabetically first remaining host becomes the default.",
		Example: "  $ copia-cli auth logout --host app.copia.io",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
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

	return cmd
}

func logoutRun(opts *LogoutOptions) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	if _, ok := cfg.Hosts[opts.Host]; !ok {
		return fmt.Errorf("not logged in to %s", opts.Host)
	}

	cfg.RemoveHost(opts.Host)

	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Logged out of %s\n", opts.Host)
	return nil
}
