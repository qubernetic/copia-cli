package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

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
	Web        bool

	EditorMode    bool
	EditorCommand string
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
milestone fails before the issue is created.

--editor opens the editor set in COPIA_EDITOR, the config key editor, VISUAL
or EDITOR on a temporary file: the title on the first line, the body after a
blank line, prefilled with --title. Empty text aborts without an issue.

--web opens the new-issue form in the browser instead, with --title and
--body filled in. No API call is made, so no token is needed; --label,
--assignee and --milestone are not applied there.`,
		Example: `  # Create an issue with a label
  $ copia-cli issue create --title "Fix sensor mapping" --label bug

  # Create an issue with a body
  $ copia-cli issue create --title "Add feature" --body "Description here"

  # Assign yourself and set a milestone by name
  $ copia-cli issue create --title "Fix sensor mapping" --assignee @me --milestone v1.0

  # Write the title and body in the editor
  $ copia-cli issue create --editor --label bug

  # Finish the issue in the browser
  $ copia-cli issue create --web --title "Fix sensor mapping"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if opts.Web {
				return openNewIssue(f, opts)
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
			if opts.EditorMode {
				opts.EditorCommand = f.Editor()
			}
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return CreateRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Issue title (required unless --editor is set)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Issue body")
	cmd.Flags().BoolVarP(&opts.EditorMode, "editor", "e", false, "Write the title and body in the text editor")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by name (repository or organization labels)")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their login. Use \"@me\" to self-assign")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Add the issue to a milestone by name or ID")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the browser to create an issue")
	cmd.MarkFlagsMutuallyExclusive("body", "editor")
	cmd.MarkFlagsMutuallyExclusive("editor", "web")

	return cmd
}

// openNewIssue opens the new-issue form in the browser, with --title and
// --body filled in through the title and body query parameters the Gitea
// web form reads. The form takes no label, assignee or milestone by name, so
// those flags only produce a warning. No API call is made.
func openNewIssue(f *cmdutil.Factory, opts *CreateOptions) error {
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}
	owner, repo, err := f.ResolveRepo()
	if err != nil {
		return err
	}

	u := cmdutil.RepoWebURL(host, owner, repo, "issues/new")
	q := url.Values{}
	if opts.Title != "" {
		q.Set("title", opts.Title)
	}
	if opts.Body != "" {
		q.Set("body", opts.Body)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	var ignored []string
	for _, flag := range []struct {
		name string
		set  bool
	}{
		{"label", len(opts.Labels) > 0},
		{"assignee", len(opts.Assignees) > 0},
		{"milestone", opts.Milestone != ""},
	} {
		if flag.set {
			ignored = append(ignored, flag.name)
		}
	}
	cmdutil.WarnWebIgnored(opts.IO, ignored...)

	return cmdutil.OpenInBrowser(opts.IO, f.Browser, u)
}

func CreateRun(opts *CreateOptions) error {
	if opts.EditorMode {
		title, body, err := cmdutil.EditTitleBody(opts.IO, opts.EditorCommand, opts.Title, opts.Body)
		if err != nil {
			return err
		}
		opts.Title, opts.Body = title, body
	}
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
