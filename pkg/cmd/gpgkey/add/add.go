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
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// AddOptions holds all inputs for the gpg-key add command.
type AddOptions struct {
	IO            *iostreams.IOStreams
	HTTPClient    *http.Client
	Host          string
	Token         string
	KeyFile       string
	SignatureFile string
}

type addRequest struct {
	ArmoredPublicKey string `json:"armored_public_key"`
	ArmoredSignature string `json:"armored_signature,omitempty"`
}

// Gitea's messages for the refusals add turns into its own.
const (
	duplicateKeyMessage = "A key with the same id already exists"
	noEmailMessage      = "None of the emails attached to the GPG key could be found."
)

// NewCmdAdd creates the `copia-cli gpg-key add` command.
func NewCmdAdd(f *cmdutil.Factory) *cobra.Command {
	return newCmdAdd(f, nil)
}

// newCmdAdd is NewCmdAdd with runF, when not nil, run instead of AddRun
// after the arguments are parsed.
func newCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{}

	cmd := &cobra.Command{
		Use:   "add [<key-file>] [-s/--signature <file>]",
		Short: "Add a GPG key to your Copia account",
		Long: `Add an ASCII-armored GPG public key to your Copia account.

With "-", or without a file when standard input is not a terminal, the key
is read from standard input.

Copia adds a key without a signature when one of its emails is a verified
email of your account. Any other key needs --signature: a detached
ASCII-armored signature, made with the key, over the token Copia gives for
your account. The token is valid for a few minutes:

  copia-cli api user/gpg_key_token | gpg -a --detach-sig > sig.asc
  copia-cli gpg-key add key.asc --signature sig.asc

Add -u <key ID> to the gpg command when gpg holds more than one secret key.
The key file and the signature file cannot both be standard input.`,
		Example: `  $ gpg --armor --export <ID> | copia-cli gpg-key add -
  $ copia-cli gpg-key add key.asc
  $ copia-cli gpg-key add key.asc --signature sig.asc`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if len(args) == 0 {
				if opts.IO.IsStdinTTY() {
					return errors.New("GPG key file missing")
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

	cmd.Flags().StringVarP(&opts.SignatureFile, "signature", "s", "", "Detached armored signature `file` over the GPG key token")

	return cmd
}

func AddRun(opts *AddOptions) error {
	if opts.KeyFile == "-" && opts.SignatureFile == "-" {
		return errors.New("the key file and the signature file cannot both be standard input")
	}

	data, err := cmdutil.ReadFile(opts.KeyFile, opts.IO.In)
	if err != nil {
		return err
	}
	key := string(data)

	// The private half never leaves the machine, so it is refused here
	// rather than by the server.
	if strings.Contains(key, "PRIVATE KEY BLOCK") {
		return errors.New("the key file holds a private key; add the public key (gpg --armor --export <ID>)")
	}
	if !strings.Contains(key, "-----BEGIN ") {
		return errors.New("the GPG key you are trying to upload might not be in ASCII-armored format.\n" +
			"Find your GPG key ID with:    gpg --list-keys\n" +
			"Then add it to your account:  gpg --armor --export <ID> | copia-cli gpg-key add -")
	}

	var signature string
	if opts.SignatureFile != "" {
		data, err := cmdutil.ReadFile(opts.SignatureFile, opts.IO.In)
		if err != nil {
			return err
		}
		signature = string(data)
		if !strings.Contains(signature, "-----BEGIN ") {
			return errors.New("the signature is not ASCII-armored; create it with gpg -a --detach-sig")
		}
	}

	body, err := json.Marshal(addRequest{ArmoredPublicKey: key, ArmoredSignature: signature})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/user/gpg_keys", opts.Host), bytes.NewReader(body))
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
		if data, err := io.ReadAll(resp.Body); err != nil || json.Unmarshal(data, &apiErr) != nil || apiErr.Message == "" {
			return fmt.Errorf("failed to add GPG key (HTTP %d)", resp.StatusCode)
		}
		switch {
		case resp.StatusCode == http.StatusUnprocessableEntity && apiErr.Message == duplicateKeyMessage:
			return errors.New("the key already exists in your account")
		case resp.StatusCode == http.StatusNotFound && strings.HasPrefix(apiErr.Message, noEmailMessage):
			// The token in Gitea's message is no secret: it only proves
			// a signature fresh and expires within minutes.
			return fmt.Errorf("failed to add GPG key (HTTP %d): %s\n"+
				"Sign the token with the key and add the key with the signature:\n"+
				"  copia-cli api user/gpg_key_token | gpg -a --detach-sig > sig.asc\n"+
				"  copia-cli gpg-key add <key-file> --signature sig.asc", resp.StatusCode, redact(apiErr.Message, key+"\n"+signature))
		}
		return fmt.Errorf("failed to add GPG key (HTTP %d): %s", resp.StatusCode, redact(apiErr.Message, key+"\n"+signature))
	}

	_, _ = fmt.Fprintln(opts.IO.Out, "GPG key added to your account")
	return nil
}

// redact replaces every line of content that is long enough to be key or
// signature material in a server message, so an error never shows the
// content of the key file or the signature file.
func redact(message, content string) string {
	for _, line := range strings.Split(content, "\n") {
		if line = strings.TrimSpace(line); len(line) >= 16 {
			message = strings.ReplaceAll(message, line, "[key content]")
		}
	}
	return message
}
