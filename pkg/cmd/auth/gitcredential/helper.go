// Package gitcredential implements `copia-cli auth git-credential`, the
// credential helper git runs to get the token of a Copia host, as gh's
// `auth git-credential` does.
package gitcredential

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CredentialOptions holds all inputs for the git-credential command.
type CredentialOptions struct {
	IO        *iostreams.IOStreams
	Operation string
	Config    func() (*config.Config, error)
	// Keyring is the OS keyring a token missing from the config file is
	// read from; nil reads the config file only.
	Keyring config.TokenStore
}

// NewCmdCredential creates the hidden `copia auth git-credential` command.
func NewCmdCredential(f *cmdutil.Factory) *cobra.Command {
	opts := &CredentialOptions{}

	cmd := &cobra.Command{
		Use:    "git-credential <get|store|erase>",
		Short:  "Implement the git credential helper protocol",
		Long:   "Answer git's credential requests with the stored token of a configured Copia host. For get with protocol https and a configured host (the port must match, a missing port is 443), it prints the login and token of the host's active account; for anything else it prints nothing. store and erase do nothing. COPIA_TOKEN and --token do not apply, because git asks for each host separately.",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Operation = args[0]
			opts.Config = f.Config
			opts.Keyring = f.Keyring
			return helperRun(opts)
		},
	}

	return cmd
}

func helperRun(opts *CredentialOptions) error {
	switch opts.Operation {
	case "store", "erase":
		// The token stays where auth login stored it; git keeps no copy.
		return nil
	case "get":
	default:
		return fmt.Errorf("git-credential: %q operation not supported", opts.Operation)
	}

	wants, err := readRequest(opts.IO)
	if err != nil {
		return err
	}
	if wants["protocol"] != "https" {
		return nil
	}
	requested, err := cmdutil.ParseHost(wants["host"])
	if err != nil {
		return nil
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host := ""
	for _, name := range cfg.HostNames() {
		configured, err := cmdutil.ParseHost(name)
		if err == nil && cmdutil.MatchesHost(configured, requested.String()) {
			host = name
			break
		}
	}
	if host == "" {
		return nil
	}

	token, _, err := config.LookupToken(cfg, host, opts.Keyring)
	if errors.Is(err, config.ErrTokenNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the token for %s: %w", host, err)
	}
	login := cfg.Hosts[host].User
	if login == "" {
		// A token of the config shape before accounts has no login; Copia
		// takes a token as the password with any user name.
		login = "token"
	}
	if user := wants["username"]; user != "" && !strings.EqualFold(user, login) {
		return nil
	}
	if !cmdutil.IsPrintableASCII(token) {
		return fmt.Errorf("invalid token for %s: it may contain only printable ASCII characters", host)
	}
	if !cmdutil.IsPrintableASCII(login) {
		return fmt.Errorf("invalid login for %s: it may contain only printable ASCII characters", host)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "username=%s\npassword=%s\n", login, token)
	return nil
}

// readRequest reads git's key=value lines up to the first empty line. A url
// line is split into protocol, host, path and username, as gh does.
func readRequest(io *iostreams.IOStreams) (map[string]string, error) {
	wants := map[string]string{}
	s := bufio.NewScanner(io.In)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key != "url" {
			wants[key] = value
			continue
		}
		u, err := url.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("git-credential: invalid url in the request")
		}
		wants["protocol"] = u.Scheme
		wants["host"] = u.Host
		wants["path"] = u.Path
		wants["username"] = u.User.Username()
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("git-credential: reading the request: %w", err)
	}
	return wants, nil
}
