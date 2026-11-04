package delete

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// DeleteOptions holds all inputs for the issue delete command.
type DeleteOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Confirmed  bool
}

// NewCmdDelete creates the `copia issue delete` command.
func NewCmdDelete(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{}

	cmd := &cobra.Command{
		Use:   "delete {<number> | <url>}",
		Short: "Delete an issue",
		Long: "Delete an issue by number or URL. This action is irreversible, so it needs --yes.\n\n" +
			"Gitea allows deleting an issue only to repository admins. app.copia.io refuses the request\n" +
			"with HTTP 405; close the issue with `copia-cli issue close` there.",
		Example: `  # Delete an issue
  $ copia-cli issue delete 12 --yes

  # Delete by URL
  $ copia-cli issue delete https://app.copia.io/my-org/my-repo/issues/12 --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := shared.ParseIssueArg(args[0])
			if err != nil {
				return err
			}
			opts.Number = ref.Number
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			var owner, repo string
			if ref.Host == "" {
				owner, repo, err = f.ResolveRepo()
				if err != nil {
					return err
				}
			}
			opts.Owner, opts.Repo, err = ref.Resolve(host, owner, repo)
			if err != nil {
				return err
			}
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return DeleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting")

	return cmd
}

func DeleteRun(opts *DeleteOptions) error {
	issue := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)
	if !opts.Confirmed {
		return fmt.Errorf("deleting issue %s is irreversible; use --yes to confirm", issue)
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
	case http.StatusForbidden:
		return fmt.Errorf("cannot delete issue %s: permission denied (HTTP 403); deleting an issue needs admin rights on the repository", issue)
	case http.StatusNotFound:
		return fmt.Errorf("issue %s not found", issue)
	case http.StatusMethodNotAllowed:
		return fmt.Errorf("%s does not allow deleting issues (HTTP 405); close the issue with `copia-cli issue close` instead", opts.Host)
	default:
		return fmt.Errorf("failed to delete issue (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Deleted issue %s\n", issue)
	return nil
}
