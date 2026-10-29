package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CreateOptions holds all inputs for the label create command.
type CreateOptions struct {
	IO          *iostreams.IOStreams
	HTTPClient  *http.Client
	Host        string
	Token       string
	Owner       string
	Repo        string
	Name        string
	Color       string
	Description string
}

type createRequest struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
}

// NewCmdCreate creates the `copia label create` command.
func NewCmdCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a label",
		Long:  "Create a new label in a Copia repository. The --name and --color flags are required.",
		Example: `  # Create a label
  $ copia-cli label create --name bug --color "#e11d48"

  # Create a label with a description
  $ copia-cli label create --name feature --color "#0969da" --description "New feature"`,
		RunE: func(cmd *cobra.Command, args []string) error {
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

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Label name (required)")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "Label color in hex (e.g. #e11d48)")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Label description")

	return cmd
}

func CreateRun(opts *CreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("name required")
	}

	payload := createRequest{
		Name:        opts.Name,
		Color:       opts.Color,
		Description: opts.Description,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels", opts.Host, opts.Owner, opts.Repo)
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
		return fmt.Errorf("failed to create label (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Label %q created\n", opts.Name)
	return nil
}
