package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CreateOptions holds all inputs for the issue create command.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Title      string
	Body       string
	Labels     []string
	Assignees  []string
	Milestone  string
}

type createRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Labels    []int64  `json:"labels,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Milestone int64    `json:"milestone,omitempty"`
}

type createResponse struct {
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
}

// NewCmdCreate creates the `copia issue create` command.
func NewCmdCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Long: `Create an issue on Copia. The title is required; body, labels, assignees and
milestone are optional.

--assignee takes a login and can be repeated; @me stands for the authenticated
user. --milestone takes a milestone name or its numeric ID; an unknown
milestone fails before the issue is created.`,
		Example: `  # Create an issue with a label
  $ copia-cli issue create --title "Fix sensor mapping" --label bug

  # Create an issue with a body
  $ copia-cli issue create --title "Add feature" --body "Description here"

  # Assign yourself and set a milestone by name
  $ copia-cli issue create --title "Fix sensor mapping" --assignee @me --milestone v1.0`,
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
			opts.HTTPClient = &http.Client{}
			return CreateRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Issue title (required)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Issue body")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by name (repository or organization labels)")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their login. Use \"@me\" to self-assign")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Add the issue to a milestone by name or ID")

	return cmd
}

func CreateRun(opts *CreateOptions) error {
	if opts.Title == "" {
		return fmt.Errorf("title required")
	}

	payload := createRequest{
		Title: opts.Title,
		Body:  opts.Body,
	}
	if len(opts.Labels) > 0 {
		// The API takes label IDs, not names.
		ids, err := shared.ResolveLabelIDs(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Labels)
		if err != nil {
			return err
		}
		payload.Labels = ids
	}
	if len(opts.Assignees) > 0 {
		assignees, err := shared.ResolveAssignees(opts.HTTPClient, opts.Host, opts.Token, opts.Assignees)
		if err != nil {
			return err
		}
		payload.Assignees = assignees
	}
	if opts.Milestone != "" {
		id, err := shared.ResolveMilestoneID(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Milestone)
		if err != nil {
			return err
		}
		payload.Milestone = id
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues", opts.Host, opts.Owner, opts.Repo)
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
		return fmt.Errorf("failed to create issue (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result createResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Created issue #%d: %s\n%s\n", result.Number, result.Title, result.HTMLURL)
	return nil
}
