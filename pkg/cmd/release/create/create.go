package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type CreateOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Tag        string
	Title      string
	Notes      string
	Draft      bool
	Prerelease bool
}

type createRequest struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name,omitempty"`
	Body       string `json:"body,omitempty"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type createResponse struct {
	ID      int64  `json:"id"`
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

func NewCmdCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create <tag>",
		Short: "Create a release",
		Long:  "Create a new release for a Copia repository. A tag name is required. Use --draft or --prerelease to control release visibility.",
		Example: `  # Create a release
  $ copia-cli release create v1.0.0 --title "Release 1.0.0" --notes "Changelog here"

  # Create a draft prerelease
  $ copia-cli release create v2.0.0-rc.1 --draft --prerelease`,
		Args: cobra.ExactArgs(1),
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
			return CreateRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Release title")
	cmd.Flags().StringVarP(&opts.Notes, "notes", "n", "", "Release notes")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Create as draft")
	cmd.Flags().BoolVar(&opts.Prerelease, "prerelease", false, "Mark as pre-release")

	return cmd
}

func CreateRun(opts *CreateOptions) error {
	if opts.Tag == "" {
		return fmt.Errorf("tag required")
	}

	payload := createRequest{
		TagName:    opts.Tag,
		Name:       opts.Title,
		Body:       opts.Notes,
		Draft:      opts.Draft,
		Prerelease: opts.Prerelease,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases", opts.Host, opts.Owner, opts.Repo)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
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
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create release (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result createResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Created release %s: %s\n%s\n", result.TagName, result.Name, result.HTMLURL)
	return nil
}
