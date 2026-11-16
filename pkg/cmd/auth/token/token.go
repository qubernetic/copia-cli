// Package token implements `copia-cli auth token`.
package token

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdToken creates the `copia auth token` command.
func NewCmdToken(f *cmdutil.Factory) *cobra.Command {
	var user string

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token copia-cli uses for a host and account",
		Long:  "Print the authentication token for an account on a Copia host, followed by a newline. Without --host the host is COPIA_HOST, then the default host. Without --user it prints the token the other commands use: --token, then COPIA_TOKEN when COPIA_HOST or --host names the host, then the stored token of the host's active account. With --user it prints the stored token of that account only, from the config file or the OS keyring.",
		Example: "  $ copia-cli auth token\n  $ copia-cli auth token --host app.copia.io --user john",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken(f, user)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(f.IOStreams.Out, token)
			return nil
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "The account to output the token for")

	return cmd
}

// resolveToken returns the token of the resolved host: the one ResolveAuth
// picks, or the stored token of user when user is set. No error holds the
// token.
func resolveToken(f *cmdutil.Factory, user string) (string, error) {
	if user == "" {
		_, token, err := f.ResolveAuth()
		if errors.Is(err, cmdutil.ErrNoToken) {
			host, hostErr := f.ResolveHost()
			if hostErr != nil {
				return "", hostErr
			}
			return "", fmt.Errorf("no token found for %s", host)
		}
		return token, err
	}

	host, err := f.ResolveHost()
	if err != nil {
		return "", err
	}
	cfg, err := f.Config()
	if err != nil {
		return "", err
	}
	token, _, err := config.LookupUserToken(cfg, host, user, f.Keyring)
	switch {
	case errors.Is(err, config.ErrTokenNotFound):
		return "", fmt.Errorf("no token found for %s account %s", host, user)
	case err != nil:
		return "", fmt.Errorf("reading the token for %s account %s: %w", host, user, err)
	}
	return token, nil
}
