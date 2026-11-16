package deleteasset

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/release/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// DeleteAssetOptions holds all inputs for the release delete-asset command.
type DeleteAssetOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Tag        string
	AssetName  string
	Confirmed  bool
}

// NewCmdDeleteAsset creates the `copia-cli release delete-asset` command.
func NewCmdDeleteAsset(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteAssetOptions{}

	cmd := &cobra.Command{
		Use:   "delete-asset <tag> <asset-name>",
		Short: "Delete an asset from a release",
		Long:  "Delete an asset from a release of a Copia repository. This action is irreversible, so it needs --yes.",
		Example: `  # Delete the asset app.zip from the release v1.0.0
  $ copia-cli release delete-asset v1.0.0 app.zip --yes`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Tag = args[0]
			opts.AssetName = args[1]

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
			return DeleteAssetRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Confirmed, "yes", "y", false, "Confirm deletion without prompting")

	return cmd
}

// DeleteAssetRun looks the asset up by name in the release Tag and deletes it
// with DELETE /repos/{owner}/{repo}/releases/{id}/assets/{asset_id}.
func DeleteAssetRun(opts *DeleteAssetOptions) error {
	if !opts.Confirmed {
		return fmt.Errorf("deleting asset %s from release %s is irreversible; use --yes to confirm", opts.AssetName, opts.Tag)
	}

	release, err := shared.FetchRelease(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Tag)
	if err != nil {
		return err
	}

	notFound := fmt.Errorf("asset %s not found in release %s", opts.AssetName, release.TagName)
	var asset *shared.Asset
	for i := range release.Assets {
		if release.Assets[i].Name == opts.AssetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		return notFound
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/%d/assets/%d",
		opts.Host, opts.Owner, opts.Repo, release.ID, asset.ID)
	req, err := http.NewRequest("DELETE", u, nil)
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
		return notFound
	default:
		return fmt.Errorf("failed to delete asset (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Deleted asset %s from release %s\n", opts.AssetName, release.TagName)
	return nil
}
