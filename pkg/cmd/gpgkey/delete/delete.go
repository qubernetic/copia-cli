package delete

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	gpgkeylist "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// DeleteOptions holds all inputs for the gpg-key delete command.
type DeleteOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	KeyID      string
}

// NewCmdDelete creates the `copia-cli gpg-key delete` command.
func NewCmdDelete(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{}
	var confirmed bool

	cmd := &cobra.Command{
		Use:   "delete <key-id>",
		Short: "Delete a GPG key from your Copia account",
		Long: `Delete a GPG key from your Copia account.

The key is named by its GPG key ID, as gpg-key list shows it, in any case.
There is no prompt, so --yes is required.`,
		Example: `  $ copia-cli gpg-key delete 3AA5C34371567BD2 --yes`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("cannot delete: key id required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirmed {
				return errors.New("--yes required when not running interactively")
			}
			opts.IO = f.IOStreams
			opts.KeyID = args[0]

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return DeleteRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

// DeleteRun looks the key up by its GPG key ID on every page of GET
// /user/gpg_keys and deletes it with DELETE /user/gpg_keys/{id}; Gitea
// answers 204 to a delete of an unknown id, so the lookup comes first. The
// command has checked --yes before.
func DeleteRun(opts *DeleteOptions) error {
	keys, err := gpgkeylist.FetchKeys(opts.HTTPClient, opts.Host, opts.Token)
	if err != nil {
		return err
	}

	var id int64
	for _, k := range keys {
		if strings.EqualFold(k.KeyID, opts.KeyID) {
			id = k.ID
			break
		}
	}
	if id == 0 {
		return fmt.Errorf("unable to delete GPG key %s: either the GPG key is not found or it is not owned by you", opts.KeyID)
	}

	req, err := http.NewRequest("DELETE", fmt.Sprintf("https://%s/api/v1/user/gpg_keys/%d", opts.Host, id), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		if message := cmdutil.ReadAPIMessage(resp.Body); message != "" {
			return fmt.Errorf("failed to delete GPG key (HTTP %d): %s", resp.StatusCode, message)
		}
		return fmt.Errorf("failed to delete GPG key (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "GPG key %s deleted from your account\n", opts.KeyID)
	return nil
}
