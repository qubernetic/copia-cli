package status

import (
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
}

// NewCmdStatus creates the `copia auth status` command.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{}

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "View authentication status",
		Long:    "Display the authentication state for each configured Copia host, including the active user, the masked token and token validity. Use --show-token to print the token itself.",
		Example: "  $ copia-cli auth status\n  $ copia-cli auth status --show-token",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.HTTPClient = &http.Client{}
			opts.Host = f.Host
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

	for host, hc := range cfg.Hosts {
		if opts.Host != "" && host != opts.Host {
			continue
		}
		tokenStatus := "Token valid"
		if opts.HTTPClient != nil {
			url := fmt.Sprintf("https://%s/api/v1/user", host)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				tokenStatus = fmt.Sprintf("Error: %v", err)
			} else {
				req.Header.Set("Authorization", "token "+hc.Token)
				resp, err := opts.HTTPClient.Do(req)
				if err != nil {
					tokenStatus = fmt.Sprintf("Error: %v", err)
				} else {
					defer func() { _ = resp.Body.Close() }()
					if resp.StatusCode != http.StatusOK {
						tokenStatus = fmt.Sprintf("Token invalid (HTTP %d)", resp.StatusCode)
					}
				}
			}
		}

		token := maskToken(hc.Token)
		if opts.ShowToken {
			token = hc.Token
		}

		_, _ = fmt.Fprintf(opts.IO.Out, "%s\n  User: %s\n  Token: %s\n  %s\n", host, hc.User, token, tokenStatus)
	}

	return nil
}

// maskToken replaces every character of the token with an asterisk. Copia
// tokens carry no prefix, so unlike gh nothing of the token stays visible.
func maskToken(token string) string {
	return strings.Repeat("*", utf8.RuneCountInString(token))
}
