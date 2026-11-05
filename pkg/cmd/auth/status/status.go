package status

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// StatusOptions holds all inputs for the status command.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	ConfigPath string
	HTTPClient *http.Client
	Host       string // filter to specific host (optional)
	ShowToken  bool   // print the token instead of its mask
	// Keyring is the OS keyring a token missing from the config file is
	// read from; nil reads the config file only.
	Keyring config.TokenStore
}

// NewCmdStatus creates the `copia auth status` command.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{}

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "View authentication status",
		Long:    "Display the authentication state for each configured Copia host and each of its accounts, the active account first: the user, whether it is the host's active account (the one the other commands use), the masked token with where it is stored (keyring or config file) and token validity. The hosts are listed in alphabetical order and the default host is marked with (default). Use --show-token to print the token itself.",
		Example: "  $ copia-cli auth status\n  $ copia-cli auth status --show-token",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.HTTPClient = cmdutil.NewHTTPClient()
			opts.Host = f.Host
			opts.Keyring = f.Keyring
			return StatusRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.ShowToken, "show-token", false, "Display the authentication token")

	return cmd
}

func StatusRun(opts *StatusOptions) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	if len(cfg.Hosts) == 0 {
		return fmt.Errorf("not logged in to any Copia instance. Run 'copia auth login'")
	}

	// If --host is specified, only show that host
	if opts.Host != "" {
		if _, ok := cfg.Hosts[opts.Host]; !ok {
			return fmt.Errorf("not logged in to %s", opts.Host)
		}
	}

	// Piping the token on purpose is valid, so warn instead of refusing.
	if opts.ShowToken && !opts.IO.IsStdoutTTY() {
		_, _ = fmt.Fprintln(opts.IO.ErrOut, "warning: printing the token to a non-terminal output; it may end up in a log or a file")
	}

	defaultHost, _ := cfg.DefaultHost()
	for _, host := range cfg.HostNames() {
		if opts.Host != "" && host != opts.Host {
			continue
		}
		label := host
		if host == defaultHost {
			label += " (default)"
		}
		_, _ = fmt.Fprintln(opts.IO.Out, label)

		hc := cfg.Hosts[host]
		// The active account comes first, then the others alphabetically.
		logins := []string{hc.User}
		for _, login := range hc.Logins() {
			if login != hc.User {
				logins = append(logins, login)
			}
		}
		for _, login := range logins {
			printAccount(opts, cfg, host, login, login == hc.User)
		}
	}

	return nil
}

// printAccount prints the user, whether it is the host's active account, the
// stored token with its source and whether the server accepts it.
func printAccount(opts *StatusOptions, cfg *config.Config, host, login string, active bool) {
	var stored, source string
	var err error
	if active {
		stored, source, err = config.LookupToken(cfg, host, opts.Keyring)
	} else {
		stored, source, err = config.LookupUserToken(cfg, host, login, opts.Keyring)
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "  User: %s\n  Active account: %t\n", login, active)
	if err != nil {
		tokenStatus := "No token stored"
		if !errors.Is(err, config.ErrTokenNotFound) {
			tokenStatus = fmt.Sprintf("Error: %v", err)
		}
		_, _ = fmt.Fprintf(opts.IO.Out, "  Token: none\n  %s\n", tokenStatus)
		return
	}

	token := maskToken(stored)
	if opts.ShowToken {
		token = stored
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "  Token: %s (%s)\n  %s\n", token, source, checkToken(opts.HTTPClient, host, stored))
}

// checkToken asks the server for the token's user and describes the answer.
func checkToken(client *http.Client, host, token string) string {
	if client == nil {
		return "Token valid"
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/api/v1/user", host), nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	req.Header.Set("Authorization", "token "+token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Token invalid (HTTP %d)", resp.StatusCode)
	}
	return "Token valid"
}

// maskToken replaces every character of the token with an asterisk. Copia
// tokens carry no prefix, so unlike gh nothing of the token stays visible.
func maskToken(token string) string {
	return strings.Repeat("*", utf8.RuneCountInString(token))
}
