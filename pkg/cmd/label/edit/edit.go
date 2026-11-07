package edit

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

// EditOptions holds all inputs for the label edit command.
type EditOptions struct {
	IO          *iostreams.IOStreams
	HTTPClient  *http.Client
	Host        string
	Token       string
	Owner       string
	Repo        string
	Name        string
	NewName     string
	Color       string
	Description string
}

type labelEntry struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type editRequest struct {
	Name        string `json:"name,omitempty"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewCmdEdit creates the `copia label edit` command.
func NewCmdEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &EditOptions{}

	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a label",
		Long: `Update a label in a Copia repository.

A label can be renamed using the --name flag.

The label color needs to be a 6 character hex value; a leading # is dropped.`,
		Example: `  # Update the color of the bug label
  $ copia-cli label edit bug --color FF0000

  # Rename and edit the description of the bug label
  $ copia-cli label edit bug --name big-bug --description "Bigger than normal bug"`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("cannot update label: name argument required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Name = args[0]
			if err := validate(opts); err != nil {
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
			return EditRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the label")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "Color of the label")
	cmd.Flags().StringVarP(&opts.NewName, "name", "n", "", "New name of the label")

	return cmd
}

// validate strips a leading # from the color and requires at least one
// change, as gh does.
func validate(opts *EditOptions) error {
	opts.Color = strings.TrimPrefix(opts.Color, "#")
	if opts.Description == "" && opts.Color == "" && opts.NewName == "" {
		return errors.New("specify at least one of --color, --description, or --name")
	}
	return nil
}

func decodeLabels(body []byte) ([]labelEntry, error) {
	var labels []labelEntry
	err := json.Unmarshal(body, &labels)
	return labels, err
}

// EditRun looks the label up by name on every page of the label list and
// sends the changes with PATCH /repos/{owner}/{repo}/labels/{id}.
func EditRun(opts *EditOptions) error {
	if err := validate(opts); err != nil {
		return err
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
	// Gitea does not refuse a second label of the same name, so the
	// conflict gh reports from the API is checked here.
	for _, l := range labels {
		if opts.NewName != "" && l.Name == opts.NewName && l.ID != target.ID {
			return fmt.Errorf("label with name %q already exists", opts.NewName)
		}
	}

	body, err := json.Marshal(editRequest{
		Name:        opts.NewName,
		Color:       opts.Color,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/%d", labelsURL, target.ID), bytes.NewReader(body))
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

	switch {
	case resp.StatusCode == http.StatusOK:
	case opts.NewName != "" && (resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity):
		return fmt.Errorf("label with name %q already exists", opts.NewName)
	default:
		return fmt.Errorf("failed to update label (HTTP %d)", resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Label %q updated in %s/%s\n", opts.Name, opts.Owner, opts.Repo)
	return nil
}
