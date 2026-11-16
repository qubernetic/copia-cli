package add

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// AddOptions holds all inputs for the gpg-key add command.
type AddOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	KeyFile    string
}

// addRequest leaves out Gitea's armored_signature: the signature is made
// over the token of GET /user/gpg_key_token, a route Copia does not have.
type addRequest struct {
	ArmoredPublicKey string `json:"armored_public_key"`
}

// Gitea's messages for the refusals add turns into its own.
const (
	duplicateKeyMessage = "A key with the same id already exists"
	noEmailMessage      = "None of the emails attached to the GPG key could be found."
)

// publicKeyHeader is the only armor header a key file may hold.
const publicKeyHeader = "-----BEGIN PGP PUBLIC KEY BLOCK-----"

// NewCmdAdd creates the `copia-cli gpg-key add` command.
func NewCmdAdd(f *cmdutil.Factory) *cobra.Command {
	return newCmdAdd(f, nil)
}

// newCmdAdd is NewCmdAdd with runF, when not nil, run instead of AddRun
// after the arguments are parsed.
func newCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{}

	cmd := &cobra.Command{
		Use:   "add [<key-file>]",
		Short: "Add a GPG key to your Copia account",
		Long: `Add an ASCII-armored GPG public key to your Copia account.

With "-", or without a file when standard input is not a terminal, the key
is read from standard input.

Copia adds a key only when one of its emails is a verified email of your
account.`,
		Example: `  $ gpg --armor --export <ID> | copia-cli gpg-key add -
  $ copia-cli gpg-key add key.asc`,
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

	return cmd
}

func AddRun(opts *AddOptions) error {
	data, err := cmdutil.ReadFile(opts.KeyFile, opts.IO.In)
	if err != nil {
		return err
	}
	key := string(data)

	// The private half never leaves the machine, so only public key blocks
	// are sent; anything else is refused here rather than by the server.
	switch cmdutil.CheckArmor(key, publicKeyHeader) {
	case cmdutil.ArmorPrivate:
		return errors.New("the key file holds a private key; add the public key (gpg --armor --export <ID>)")
	case cmdutil.ArmorOther:
		return errors.New("the GPG key you are trying to upload might not be in ASCII-armored format.\n" +
			"Find your GPG key ID with:    gpg --list-keys\n" +
			"Then add it to your account:  gpg --armor --export <ID> | copia-cli gpg-key add -")
	}

	body, err := json.Marshal(addRequest{ArmoredPublicKey: key})
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
		message := cmdutil.ReadAPIMessage(resp.Body)
		switch {
		case message == "" || quotesKey(message, key):
			return fmt.Errorf("failed to add GPG key (HTTP %d)", resp.StatusCode)
		case resp.StatusCode == http.StatusUnprocessableEntity && message == duplicateKeyMessage:
			return errors.New("the key already exists in your account")
		case resp.StatusCode == http.StatusNotFound && strings.HasPrefix(message, noEmailMessage):
			// The rest of Gitea's message offers to sign a token, which
			// Copia gives no route for, so only its first sentence is shown.
			return fmt.Errorf("failed to add GPG key (HTTP %d): %s\n"+
				"Copia adds a GPG key only when one of its emails is a verified email of your account;\n"+
				"add and verify that email in your Copia settings, then add the key again", resp.StatusCode, noEmailMessage)
		}
		return fmt.Errorf("failed to add GPG key (HTTP %d): %s", resp.StatusCode, message)
	}

	_, _ = fmt.Fprintln(opts.IO.Out, "GPG key added to your account")
	return nil
}

// quoteWindow is the shortest run of key material quotesKey looks for.
const quoteWindow = 16

// quotesKey reports whether message holds quoteWindow bytes in a row of the
// key material in key, both compared without whitespace, so a key the server
// quotes re-wrapped or joined is found as well. The key material is every
// line but the armor lines (-----) and armor headers (Key: value), which are
// no secret. An error then shows no part of the key file.
func quotesKey(message, key string) bool {
	var material strings.Builder
	for _, line := range strings.Split(key, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-----") || strings.Contains(line, ": ") {
			continue
		}
		material.WriteString(line)
	}
	body := stripSpace(material.String())
	message = stripSpace(message)
	for i := 0; i+quoteWindow <= len(body); i++ {
		if strings.Contains(message, body[i:i+quoteWindow]) {
			return true
		}
	}
	return false
}

// stripSpace returns s without any white space.
func stripSpace(s string) string {
	return strings.Join(strings.Fields(s), "")
}
