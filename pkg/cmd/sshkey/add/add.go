package add

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/sshkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// AddOptions holds all inputs for the ssh-key add command.
type AddOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	KeyFile    string
	Title      string
}

type addRequest struct {
	Title string `json:"title"`
	Key   string `json:"key"`
}

// NewCmdAdd creates the `copia-cli ssh-key add` command.
func NewCmdAdd(f *cmdutil.Factory) *cobra.Command {
	return newCmdAdd(f, nil)
}

// newCmdAdd is NewCmdAdd with runF, when not nil, run instead of AddRun
// after the arguments are parsed.
func newCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{}

	cmd := &cobra.Command{
		Use:   "add [<key-file>]",
		Short: "Add an SSH key to your Copia account",
		Long: `Add an SSH public key to your Copia account.

The key file is a public key in the OpenSSH format, such as ~/.ssh/id_ed25519.pub.
With "-", or without a file when standard input is not a terminal, the key
is read from standard input.

The title defaults to the key's comment, the third field of the key line.
A key without a comment needs --title, since Copia requires a title.

A key that is already on your account is not added again.`,
		Example: `  $ copia-cli ssh-key add ~/.ssh/id_ed25519.pub
  $ copia-cli ssh-key add ~/.ssh/id_ed25519.pub --title "work laptop"
  $ cat key.pub | copia-cli ssh-key add --title ci`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if len(args) == 0 {
				if opts.IO.IsStdinTTY() {
					return errors.New("public key file missing")
				}
				opts.KeyFile = "-"
			} else {
				opts.KeyFile = args[0]
			}
			if runF != nil {
				return runF(opts)
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return AddRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title for the new key (default: the key's comment)")

	return cmd
}

func AddRun(opts *AddOptions) error {
	data, err := cmdutil.ReadFile(opts.KeyFile, opts.IO.In)
	if err != nil {
		return err
	}
	content := string(data)

	// The private half never leaves the machine, so it is refused here
	// rather than by the server.
	if strings.Contains(content, "PRIVATE KEY") {
		return errors.New("the key file holds a private key; add the public key (the .pub file)")
	}
	fields := strings.Fields(content)
	if len(fields) < 2 {
		return errors.New("provided key is not in a valid format")
	}
	title := opts.Title
	if title == "" && len(fields) > 2 {
		title = fields[2]
	}
	if title == "" {
		return errors.New("a title is required: use --title")
	}

	keys, err := list.FetchKeys(opts.HTTPClient, opts.Host, opts.Token)
	if err != nil {
		return err
	}
	keyToCompare := fields[0] + " " + fields[1]
	for _, k := range keys {
		if k.Key == keyToCompare {
			_, _ = fmt.Fprintln(opts.IO.Out, "Public key already exists on your account")
			return nil
		}
	}

	body, err := json.Marshal(addRequest{Title: title, Key: content})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/user/keys", opts.Host), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		var apiErr struct {
			Message string `json:"message"`
		}
		if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return fmt.Errorf("failed to add SSH key (HTTP %d): %s", resp.StatusCode, redactKey(apiErr.Message, fields))
		}
		return fmt.Errorf("failed to add SSH key (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintln(opts.IO.Out, "Public key added to your account")
	return nil
}

// redactKey replaces every field of the key file that is long enough to be
// key material in a server message, so an error never shows the key's
// content.
func redactKey(message string, fields []string) string {
	for _, f := range fields {
		if len(f) >= 16 {
			message = strings.ReplaceAll(message, f, "[key content]")
		}
	}
	return message
}
