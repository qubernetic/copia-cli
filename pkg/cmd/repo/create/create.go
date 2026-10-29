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
	IO          *iostreams.IOStreams
	HTTPClient  *http.Client
	Host        string
	Token       string
	Name        string
	Description string
	Org         string
	Private     bool
}

type createRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Private     bool   `json:"private"`
}

type createResponse struct {
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	CloneURL string `json:"clone_url"`
}

func NewCmdCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a repository",
		Long:  "Create a new Copia repository. By default the repository is created under the authenticated user. Use --org to create it under an organization.",
		Example: `  # Create a repository
  $ copia-cli repo create my-repo

  # Create a private repository under an organization
  $ copia-cli repo create my-repo --org my-org --private

  # Create with a description
  $ copia-cli repo create my-repo --description "PLC project"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return CreateRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Repository description")
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "Create in organization")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "Make repository private")

	return cmd
}

func CreateRun(opts *CreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("name required")
	}

	payload := createRequest{
		Name:        opts.Name,
		Description: opts.Description,
		Private:     opts.Private,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := "/api/v1/user/repos"
	if opts.Org != "" {
		endpoint = fmt.Sprintf("/api/v1/orgs/%s/repos", opts.Org)
	}

	url := fmt.Sprintf("https://%s%s", opts.Host, endpoint)
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
		return fmt.Errorf("failed to create repository (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result createResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Created repository %s\n%s\n", result.FullName, result.HTMLURL)
	return nil
}
