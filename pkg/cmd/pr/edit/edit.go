package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	prshared "github.com/qubernetic/copia-cli/pkg/cmd/pr/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// EditOptions holds all inputs for the pr edit command.
type EditOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64

	Title    string
	TitleSet bool
	Body     *string
	Base     string

	AddLabels       []string
	RemoveLabels    []string
	AddAssignees    []string
	RemoveAssignees []string
	AddReviewers    []string
	RemoveReviewers []string
	Milestone       string
	RemoveMilestone bool
}

// NewCmdEdit creates the `copia pr edit` command.
func NewCmdEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &EditOptions{}
	var body, bodyFile string

	cmd := &cobra.Command{
		Use:   "edit {<number> | <url>}",
		Short: "Edit a pull request",
		Long: `Edit the title, body, base branch, labels, assignees, reviewers or
milestone of a Copia pull request by number or URL.

--body-file reads the body from a file, "-" from standard input; it cannot
be combined with --body. An empty --body clears the body.

--add-label and --remove-label take label names of the repository or its
organization; an unknown name fails before the pull request is changed.
--add-assignee and --remove-assignee take logins, "@me" is the authenticated
user. Labels and assignees are read first and the changed lists are sent
whole; removing a value the pull request does not have changes nothing.

--add-reviewer and --remove-reviewer take a login or an org/team name.

--milestone takes a milestone name or its numeric ID; an unknown milestone
fails before the pull request is changed. --remove-milestone clears it.

Adding and removing the same value, or --milestone with --remove-milestone,
fails before any request. On success the URL of the pull request is printed.`,
		Example: `  # Change the title and the base branch
  $ copia-cli pr edit 7 --title "Add conveyor interlock" --base develop

  # Replace the body from a file, or from standard input with -
  $ copia-cli pr edit 7 --body-file description.md

  # Change labels, assignees and reviewers
  $ copia-cli pr edit 7 --add-label plc --remove-label wip --add-assignee @me
  $ copia-cli pr edit 7 --add-reviewer jane --remove-reviewer my-org/controls

  # Set or clear the milestone
  $ copia-cli pr edit https://app.copia.io/my-org/my-repo/pulls/7 --milestone v1.0
  $ copia-cli pr edit 7 --remove-milestone`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := prshared.ParsePRArg(args[0])
			if err != nil {
				return err
			}
			opts.Number = ref.Number
			opts.IO = f.IOStreams
			opts.TitleSet = cmd.Flags().Changed("title")

			if cmd.Flags().Changed("body") {
				opts.Body = &body
			}
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				s := string(b)
				opts.Body = &s
			}
			if err := validate(opts); err != nil {
				return err
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			var owner, repo string
			if ref.Host == "" {
				owner, repo, err = f.ResolveRepo()
				if err != nil {
					return err
				}
			}
			opts.Owner, opts.Repo, err = ref.Resolve(host, owner, repo)
			if err != nil {
				return err
			}
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return EditRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Set the new title")
	cmd.Flags().StringVarP(&body, "body", "b", "", "Set the new body")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "Change the base `branch` for this pull request")
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.RemoveLabels, "remove-label", nil, "Remove labels by `name`")
	cmd.Flags().StringSliceVar(&opts.AddAssignees, "add-assignee", nil, "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	cmd.Flags().StringSliceVar(&opts.RemoveAssignees, "remove-assignee", nil, "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	cmd.Flags().StringSliceVar(&opts.AddReviewers, "add-reviewer", nil, "Add reviewers by their `login` or org/team name")
	cmd.Flags().StringSliceVar(&opts.RemoveReviewers, "remove-reviewer", nil, "Remove reviewers by their `login` or org/team name")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Edit the milestone the pull request belongs to by its `name` or ID")
	cmd.Flags().BoolVar(&opts.RemoveMilestone, "remove-milestone", false, "Remove the milestone association from the pull request")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")

	return cmd
}

// EditRun changes the pull request. Every check and lookup runs before the
// first change, so a conflict or an unknown name changes nothing.
func EditRun(opts *EditOptions) error {
	if err := validate(opts); err != nil {
		return err
	}

	payload := map[string]any{}
	if opts.Title != "" {
		payload["title"] = opts.Title
	}
	if opts.Body != nil {
		payload["body"] = *opts.Body
	}
	if opts.Base != "" {
		payload["base"] = opts.Base
	}

	if opts.Milestone != "" {
		id, err := shared.ResolveMilestoneID(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Milestone)
		if err != nil {
			return err
		}
		payload["milestone"] = id
	}

	changesLabels := len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0
	var addLabelIDs, removeLabelIDs []int64
	if changesLabels {
		ids, err := shared.ResolveLabelIDs(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo,
			slices.Concat(opts.AddLabels, opts.RemoveLabels))
		if err != nil {
			return err
		}
		addLabelIDs, removeLabelIDs = ids[:len(opts.AddLabels)], ids[len(opts.AddLabels):]
	}

	changesAssignees := len(opts.AddAssignees) > 0 || len(opts.RemoveAssignees) > 0
	var addAssignees, removeAssignees []string
	if changesAssignees {
		logins, err := shared.ResolveAssignees(opts.HTTPClient, opts.Host, opts.Token,
			slices.Concat(opts.AddAssignees, opts.RemoveAssignees))
		if err != nil {
			return err
		}
		addAssignees, removeAssignees = logins[:len(opts.AddAssignees)], logins[len(opts.AddAssignees):]
	}

	// The pull request PATCH replaces the whole label and assignee lists:
	// compute the new lists from the current ones and send them whole.
	if changesLabels || changesAssignees {
		pr, err := currentPR(opts)
		if err != nil {
			return err
		}
		if changesLabels {
			payload["labels"] = change(pr.labelIDs(), addLabelIDs, removeLabelIDs)
		}
		if changesAssignees {
			payload["assignees"] = change(pr.logins(), addAssignees, removeAssignees)
		}
	}

	var htmlURL string
	if len(payload) > 0 {
		var err error
		if htmlURL, err = updatePR(opts, payload); err != nil {
			return err
		}
	}

	// The pull request PATCH ignores milestone 0; the issue PATCH of the same
	// index clears the milestone.
	if opts.RemoveMilestone {
		_, err := send(opts, "PATCH", fmt.Sprintf("issues/%d", opts.Number), map[string]int64{"milestone": 0},
			http.StatusCreated, http.StatusOK)
		if err != nil {
			return fmt.Errorf("failed to remove the milestone of pull request %s: %w", ref(opts), err)
		}
	}

	path := fmt.Sprintf("pulls/%d/requested_reviewers", opts.Number)
	if len(opts.AddReviewers) > 0 {
		if _, err := send(opts, "POST", path, reviewRequest(opts.AddReviewers), http.StatusCreated); err != nil {
			return fmt.Errorf("failed to request reviews on pull request %s: %w", ref(opts), err)
		}
	}
	if len(opts.RemoveReviewers) > 0 {
		if _, err := send(opts, "DELETE", path, reviewRequest(opts.RemoveReviewers), http.StatusNoContent); err != nil {
			return fmt.Errorf("failed to remove review requests on pull request %s: %w", ref(opts), err)
		}
	}

	if htmlURL != "" {
		_, _ = fmt.Fprintln(opts.IO.Out, htmlURL)
	} else {
		_, _ = fmt.Fprintf(opts.IO.Out, "Updated pull request %s\n", ref(opts))
	}
	return nil
}

// validate checks the options without sending a request.
func validate(opts *EditOptions) error {
	if opts.TitleSet && strings.TrimSpace(opts.Title) == "" {
		return fmt.Errorf("title cannot be blank")
	}
	if opts.Title == "" && opts.Body == nil && opts.Base == "" && opts.Milestone == "" && !opts.RemoveMilestone &&
		len(slices.Concat(opts.AddLabels, opts.RemoveLabels, opts.AddAssignees, opts.RemoveAssignees,
			opts.AddReviewers, opts.RemoveReviewers)) == 0 {
		return fmt.Errorf("nothing to edit. Use --title, --body, --body-file, --base, --add-label, --remove-label, " +
			"--add-assignee, --remove-assignee, --add-reviewer, --remove-reviewer, --milestone or --remove-milestone")
	}
	if opts.Milestone != "" && opts.RemoveMilestone {
		return fmt.Errorf("specify only one of --milestone or --remove-milestone")
	}
	for _, c := range []struct {
		what        string
		add, remove []string
	}{
		{"label", opts.AddLabels, opts.RemoveLabels},
		{"assignee", opts.AddAssignees, opts.RemoveAssignees},
		{"reviewer", opts.AddReviewers, opts.RemoveReviewers},
	} {
		for _, v := range c.add {
			if slices.Contains(c.remove, v) {
				return fmt.Errorf("%s %q cannot be both added and removed", c.what, v)
			}
		}
	}
	return nil
}

// reviewRequest is the body of the requested_reviewers endpoints. A value
// with a slash is an org/team name; Gitea takes only the team part.
func reviewRequest(names []string) map[string][]string {
	payload := map[string][]string{}
	for _, n := range names {
		if _, team, ok := strings.Cut(n, "/"); ok {
			payload["team_reviewers"] = append(payload["team_reviewers"], team)
		} else {
			payload["reviewers"] = append(payload["reviewers"], n)
		}
	}
	return payload
}

// updatePR sends PATCH /repos/{owner}/{repo}/pulls/{index} and returns the
// html_url of the pull request.
func updatePR(opts *EditOptions, payload map[string]any) (string, error) {
	data, err := send(opts, "PATCH", fmt.Sprintf("pulls/%d", opts.Number), payload, http.StatusCreated, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("failed to update pull request %s: %w", ref(opts), err)
	}
	var pr struct {
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(data, &pr)
	return pr.HTMLURL, nil
}

type currentState struct {
	Labels []struct {
		ID int64 `json:"id"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
}

func (c currentState) labelIDs() []int64 {
	ids := make([]int64, 0, len(c.Labels))
	for _, l := range c.Labels {
		ids = append(ids, l.ID)
	}
	return ids
}

func (c currentState) logins() []string {
	logins := make([]string, 0, len(c.Assignees))
	for _, a := range c.Assignees {
		logins = append(logins, a.Login)
	}
	return logins
}

// currentPR reads the labels and assignees of the pull request.
func currentPR(opts *EditOptions) (currentState, error) {
	var pr currentState
	data, err := send(opts, "GET", fmt.Sprintf("pulls/%d", opts.Number), nil, http.StatusOK)
	if err != nil {
		return pr, fmt.Errorf("failed to read pull request %s: %w", ref(opts), err)
	}
	if err := json.Unmarshal(data, &pr); err != nil {
		return pr, fmt.Errorf("failed to read pull request %s: %w", ref(opts), err)
	}
	return pr, nil
}

// change returns current without the removed values and with the added ones
// that are not already present. The result is never nil, so removing the
// last value sends an empty list.
func change[T comparable](current, add, remove []T) []T {
	out := []T{}
	for _, v := range slices.Concat(current, add) {
		if !slices.Contains(remove, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func ref(opts *EditOptions) string {
	return fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)
}

// send makes one request below /repos/{owner}/{repo}/ and returns the
// response body. A status not in ok is an error that carries the server's
// message when it sends one.
func send(opts *EditOptions, method, path string, payload any, ok ...int) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/%s", opts.Host, opts.Owner, opts.Repo, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)

	for _, s := range ok {
		if resp.StatusCode == s {
			return data, nil
		}
	}
	var apiErr struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, apiErr.Message)
	}
	return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
}
