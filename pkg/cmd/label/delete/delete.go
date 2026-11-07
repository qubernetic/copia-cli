package delete

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// DeleteOptions holds all inputs for the label delete command.
type DeleteOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Name       string
	Confirmed  bool
}

type labelEntry struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// NewCmdDelete creates the `copia label delete` command.
func NewCmdDelete(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{}

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a label from a repository",
		Long:  "Delete a label from a Copia repository. This action is irreversible, so it needs --yes.",
		Example: `  # Delete the bug label
  $ copia-cli label delete bug --yes`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("cannot delete label: name argument required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Name = args[0]

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
			return DeleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting")

	return cmd
}

func decodeLabels(body []byte) ([]labelEntry, error) {
	var labels []labelEntry
	err := json.Unmarshal(body, &labels)
	return labels, err
}

// DeleteRun looks the label up by name on every page of the label list and
// deletes it with DELETE /repos/{owner}/{repo}/labels/{id}.
func DeleteRun(opts *DeleteOptions) error {
	if !opts.Confirmed {
		return fmt.Errorf("deleting label %q from %s/%s is irreversible; use --yes to confirm", opts.Name, opts.Owner, opts.Repo)
	}

	labelsURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels", opts.Host, opts.Owner, opts.Repo)
	labels, err := cmdutil.FetchPages(opts.HTTPClient, labelsURL, opts.Token, 0, decodeLabels)
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	var target *labelEntry
	for i := range labels {
		if labels[i].Name == opts.Name {
			target = &labels[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("label %q not found in %s/%s", opts.Name, opts.Owner, opts.Repo)
	}

	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%d", labelsURL, target.ID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
	case http.StatusNotFound:
		return fmt.Errorf("label %q not found in %s/%s", opts.Name, opts.Owner, opts.Repo)
	default:
		return fmt.Errorf("failed to delete label (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Label %q deleted from %s/%s\n", opts.Name, opts.Owner, opts.Repo)
	return nil
}
