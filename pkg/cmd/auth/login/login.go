package login

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// LoginOptions holds all inputs for the login command.
type LoginOptions struct {
	IO         *iostreams.IOStreams
	Host       string
	Token      string
	ConfigPath string
	HTTPClient *http.Client
	// Keyring receives the token; nil or InsecureStorage writes it to the
	// config file instead.
	Keyring         config.TokenStore
	InsecureStorage bool
}

// NewCmdLogin creates the `copia auth login` command.
func NewCmdLogin(f *cmdutil.Factory) *cobra.Command {
	opts := &LoginOptions{}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a Copia instance",
		Long: `Authenticate with a Copia host. The default hostname is app.copia.io.

The default authentication mode is interactive, prompting for a token.
Use --host and --token for non-interactive login in CI environments.

The host becomes the default host (the config key default_host), which the
other commands use when neither --host nor COPIA_HOST is given.

The token is stored in the OS keyring (macOS Keychain, Windows Credential
Manager, the Secret Service on Linux). When no keyring is available, the
token is stored in plain text in the config file and a warning says so.
Use --insecure-storage to store it in the config file on purpose.

Token precedence: --token flag > COPIA_TOKEN env var > the stored token
(config file, then OS keyring).`,
		Example: `  # Interactive login
  $ copia-cli auth login

  # Non-interactive login (CI/agent)
  $ copia-cli auth login --host app.copia.io --token YOUR_TOKEN`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.Keyring = f.Keyring

			if f.Token != "" {
				opts.Token = f.Token
			}
			if f.Host != "" {
				opts.Host = f.Host
			}
			if opts.Host == "" {
				opts.Host = "app.copia.io"
			}

			if opts.Token == "" && opts.IO.IsStdinTTY() {
				_, _ = fmt.Fprintf(opts.IO.ErrOut, "Enter token for %s: ", opts.Host)
				scanner := bufio.NewScanner(opts.IO.In)
				if scanner.Scan() {
					opts.Token = strings.TrimSpace(scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					return err
				}
			}

			if opts.Token == "" {
				return fmt.Errorf("token required. Use --token flag or run interactively")
			}

			opts.HTTPClient = cmdutil.NewHTTPClient()
			return LoginRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Host, "host", "", "Copia instance hostname (default: app.copia.io)")
	cmd.Flags().StringVar(&opts.Token, "token", "", "Personal access token")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "Save the token in plain text in the config file instead of the OS keyring")

	return cmd
}

type userResponse struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

func LoginRun(opts *LoginOptions) error {
	url := fmt.Sprintf("https://%s/api/v1/user", opts.Host)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed for %s (HTTP %d)", opts.Host, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var user userResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return fmt.Errorf("parsing user response: %w", err)
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	var keyringErr error
	fileToken := opts.Token
	if opts.Keyring != nil && !opts.InsecureStorage {
		keyringErr = opts.Keyring.Set(opts.Host, user.Login, opts.Token)
		if keyringErr == nil {
			// A token in the keyring leaves none in the config file, which
			// also moves a token stored there by an earlier login.
			fileToken = ""
		}
	}
	// FileStore.Set changes cfg only and cannot fail; Save writes it.
	_ = config.FileStore{Config: cfg}.Set(opts.Host, user.Login, fileToken)
	cfg.DefaultHostName = opts.Host

	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}

	if keyringErr != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "warning: could not store the token in the OS keyring (%v); stored it in the config file %s instead\n", keyringErr, opts.ConfigPath)
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Logged in as %s on %s\n", user.Login, opts.Host)
	return nil
}
