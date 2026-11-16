package remove

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	collaboratoradd "github.com/qubernetic/copia-cli/pkg/cmd/repo/collaborator/add"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// RemoveOptions holds all inputs for the repo collaborator remove command.
type RemoveOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Login      string
}

// NewCmdRemove creates the `copia-cli repo collaborator remove` command.
func NewCmdRemove(f *cmdutil.Factory) *cobra.Command {
	opts := &RemoveOptions{}
	var confirmed bool

	cmd := &cobra.Command{
		Use:   "remove <login>",
		Short: "Remove a collaborator from a repository",
		Long: `Remove a user from the collaborators of a Copia repository.

There is no prompt, so --yes is required.`,
		Args: cobra.ExactArgs(1),
		Example: `  $ copia-cli repo collaborator remove monalisa --yes
  $ copia-cli repo collaborator remove monalisa -y -R my-org/my-repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirmed {
				return errors.New("--yes required when not running interactively")
			}
			opts.IO = f.IOStreams
			opts.Login = args[0]
			if err := collaboratoradd.ValidateLogin(opts.Login); err != nil {
				return err
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return err
			}
			opts.Owner = owner
			opts.Repo = repo
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return RemoveRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

// RemoveRun checks with GET /repos/{owner}/{repo}/collaborators/{login} that
// the user is a collaborator and removes it with DELETE on the same path;
// Gitea answers 204 to a delete of a non-collaborator, so the check comes
// first. The command has checked --yes before.
func RemoveRun(opts *RemoveOptions) error {
	if err := collaboratoradd.ValidateLogin(opts.Login); err != nil {
		return err
	}
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/collaborators/%s", opts.Host, url.PathEscape(opts.Owner), url.PathEscape(opts.Repo), url.PathEscape(opts.Login))

	status, message, err := send(opts, "GET", u)
	if err != nil {
		return err
	}
	switch {
	case status == http.StatusNotFound:
		return fmt.Errorf("%s is not a collaborator of %s/%s", opts.Login, opts.Owner, opts.Repo)
	case status != http.StatusNoContent && message != "":
		return fmt.Errorf("failed to check collaborator (HTTP %d): %s", status, message)
	case status != http.StatusNoContent:
		return fmt.Errorf("failed to check collaborator (HTTP %d)", status)
	}

	status, message, err = send(opts, "DELETE", u)
	if err != nil {
		return err
	}
	switch {
	case status != http.StatusNoContent && message != "":
		return fmt.Errorf("failed to remove collaborator (HTTP %d): %s", status, message)
	case status != http.StatusNoContent:
		return fmt.Errorf("failed to remove collaborator (HTTP %d)", status)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Removed %s from %s/%s\n", opts.Login, opts.Owner, opts.Repo)
	return nil
}

// send makes an authenticated request without a body and returns the
// status and, for any status but 204, Gitea's message.
func send(opts *RemoveOptions, method, u string) (int, string, error) {
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, "", nil
	}
	return resp.StatusCode, cmdutil.ReadAPIMessage(resp.Body), nil
}
