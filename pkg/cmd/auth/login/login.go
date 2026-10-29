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

Token precedence: --token flag > COPIA_TOKEN env var > config file.`,
		Example: `  # Interactive login
  $ copia-cli auth login

  # Non-interactive login (CI/agent)
  $ copia-cli auth login --host app.copia.io --token YOUR_TOKEN`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()

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

	cfg.Hosts[opts.Host] = &config.HostConfig{
		Token: opts.Token,
		User:  user.Login,
	}

	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Logged in as %s on %s\n", user.Login, opts.Host)
	return nil
}
