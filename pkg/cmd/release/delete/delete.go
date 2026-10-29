package delete

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Tag        string
}

func NewCmdDelete(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{}

	cmd := &cobra.Command{
		Use:     "delete <tag>",
		Short:   "Delete a release",
		Long:    "Delete a release from a Copia repository by tag name.",
		Example: "  $ copia-cli release delete v1.0.0",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Tag = args[0]
			opts.IO = f.IOStreams

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

	return cmd
}

func DeleteRun(opts *DeleteOptions) error {
	// First, look up release ID by tag
	lookupURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/tags/%s",
		opts.Host, opts.Owner, opts.Repo, opts.Tag)

	lookupReq, err := http.NewRequest("GET", lookupURL, nil)
	if err != nil {
		return err
	}
	lookupReq.Header.Set("Authorization", "token "+opts.Token)

	lookupResp, err := opts.HTTPClient.Do(lookupReq)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = lookupResp.Body.Close() }()

	if lookupResp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("release %s not found", opts.Tag)
	}
	if lookupResp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (HTTP %d)", lookupResp.StatusCode)
	}

	body, err := io.ReadAll(lookupResp.Body)
	if err != nil {
		return err
	}

	var release struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return err
	}

	// Delete by ID
	deleteURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/%d",
		opts.Host, opts.Owner, opts.Repo, release.ID)

	deleteReq, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return err
	}
	deleteReq.Header.Set("Authorization", "token "+opts.Token)

	deleteResp, err := opts.HTTPClient.Do(deleteReq)
	if err != nil {
		return err
	}
	_ = deleteResp.Body.Close()

	if deleteResp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete release (HTTP %d)", deleteResp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Deleted release %s\n", opts.Tag)
	return nil
}
