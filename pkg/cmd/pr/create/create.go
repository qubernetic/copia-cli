package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CreateOptions holds all inputs for the pr create command.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Title      string
	Body       string
	Base       string
	Head       string
	Assignees  []string
	Labels     []string
	Milestone  string
	Reviewers  []string
	Draft      bool
	Fill       bool
}

type createRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Base      string   `json:"base"`
	Head      string   `json:"head"`
	Assignees []string `json:"assignees,omitempty"`
	Labels    []int64  `json:"labels,omitempty"`
	Milestone int64    `json:"milestone,omitempty"`
}

type reviewRequest struct {
	Reviewers     []string `json:"reviewers,omitempty"`
	TeamReviewers []string `json:"team_reviewers,omitempty"`
}

type createResponse struct {
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
}

// NewCmdCreate creates the `copia pr create` command.
func NewCmdCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{}
	var bodyFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: `Create a pull request on Copia. The --base and --head branches must be specified
along with a title, unless --fill is set.

--fill takes the title and body from the local git history between the base
branch (origin/<base> when it exists) and the head branch, which defaults to
the current branch. One commit gives its subject as the title and its body as
the body; several commits give the branch name as the title and the commit
subjects, oldest first, as a bulleted body. An explicit --title or --body wins
over the filled value.

--body-file reads the body from a file; "-" reads standard input. It cannot be
combined with --body.

--draft marks the pull request as work in progress: Copia has no draft flag,
so the title gets the prefix "WIP: " unless it already starts with "WIP:" or
"[WIP]" (in any case).

--assignee takes a login and can be repeated; @me stands for the authenticated
user. --label takes a repository or organization label name and can be
repeated. --milestone takes a milestone name or its numeric ID. An unknown
label or milestone fails before the pull request is created.

--reviewer takes a login or an org/team name and can be repeated. Reviewers
are requested in one request after the pull request is created; if that
request fails, the pull request still exists, its URL is printed and the
command exits with an error.`,
		Example: `  # Create a pull request
  $ copia-cli pr create --title "feat: add wrapper" --base main --head feature/wrapper

  # Create a pull request with a body
  $ copia-cli pr create --title "fix: timeout" --base develop --head fix/timeout --body "Fixes #12"

  # Open a work-in-progress PR from the current branch, filled from its commits
  $ copia-cli pr create --fill --draft --base develop

  # Read the body from a file, or from standard input with -
  $ copia-cli pr create --title "docs: add manual" --head docs/manual --body-file notes.md
  $ git log -1 --format=%b | copia-cli pr create --title "fix: timeout" --head fix/timeout -F -

  # Assign yourself, add labels and a milestone, and request reviewers
  $ copia-cli pr create --title "feat: add wrapper" --head feature/wrapper \
      --assignee @me --label plc --label review --milestone v1.0 \
      --reviewer jane --reviewer my-org/controls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
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
			return CreateRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "PR title (required unless --fill is set)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "PR body")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read the body from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVar(&opts.Base, "base", "main", "Base branch")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "Head branch")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their login. Use \"@me\" to self-assign")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by name (repository or organization labels)")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Add the pull request to a milestone by name or ID")
	cmd.Flags().StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request reviews from people or teams by their login or org/team name")
	cmd.Flags().BoolVarP(&opts.Draft, "draft", "d", false, "Mark the pull request as work in progress (WIP: title prefix)")
	cmd.Flags().BoolVarP(&opts.Fill, "fill", "f", false, "Use commit info between the base and head branches for the title and body")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")

	return cmd
}

func CreateRun(opts *CreateOptions) error {
	if opts.Fill {
		if err := fill(opts); err != nil {
			return err
		}
	}
	if opts.Title == "" {
		return fmt.Errorf("title required")
	}

	title := opts.Title
	if opts.Draft && !hasWIPPrefix(title) {
		title = "WIP: " + title
	}

	payload := createRequest{
		Title: title,
		Body:  opts.Body,
		Base:  opts.Base,
		Head:  opts.Head,
	}
	if len(opts.Assignees) > 0 {
		assignees, err := shared.ResolveAssignees(opts.HTTPClient, opts.Host, opts.Token, opts.Assignees)
		if err != nil {
			return err
		}
		payload.Assignees = assignees
	}
	if len(opts.Labels) > 0 {
		// The API takes label IDs, not names.
		ids, err := shared.ResolveLabelIDs(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Labels)
		if err != nil {
			return err
		}
		payload.Labels = ids
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

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls", opts.Host, opts.Owner, opts.Repo)
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
		return fmt.Errorf("failed to create PR (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result createResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Created PR #%d: %s\n%s\n", result.Number, result.Title, result.HTMLURL)

	if len(opts.Reviewers) > 0 {
		return requestReviewers(opts, result.Number)
	}
	return nil
}

// hasWIPPrefix reports whether title already starts with one of Gitea's
// default work-in-progress prefixes, which Gitea matches case-insensitively.
func hasWIPPrefix(title string) bool {
	upper := strings.ToUpper(title)
	return strings.HasPrefix(upper, "WIP:") || strings.HasPrefix(upper, "[WIP]")
}

// requestReviewers asks for all reviews in one request. A value with a slash
// is an org/team name; Gitea takes only the team part.
func requestReviewers(opts *CreateOptions, number int64) error {
	var payload reviewRequest
	for _, r := range opts.Reviewers {
		if _, team, ok := strings.Cut(r, "/"); ok {
			payload.TeamReviewers = append(payload.TeamReviewers, team)
		} else {
			payload.Reviewers = append(payload.Reviewers, r)
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d/requested_reviewers", opts.Host, opts.Owner, opts.Repo, number)
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
		return fmt.Errorf("failed to request reviewers (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
