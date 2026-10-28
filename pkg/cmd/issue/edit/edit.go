package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type EditOptions struct {
	IO              *iostreams.IOStreams
	HTTPClient      *http.Client
	Host            string
	Token           string
	Owner           string
	Repo            string
	Number          int64
	Title           string
	Body            string
	AddLabels       []string
	Assignees       []string
	AddAssignees    []string
	RemoveAssignees []string
	Milestone       string
}

func NewCmdEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &EditOptions{}

	cmd := &cobra.Command{
		Use:   "edit <number>",
		Short: "Edit an issue",
		Long: `Edit an issue's title, body, labels, assignees, or milestone.

--assignee replaces the whole assignee list. --add-assignee and
--remove-assignee change it: the current assignees are read first, and
removing a login the issue does not have changes nothing. --assignee cannot
be combined with --add-assignee or --remove-assignee.

--milestone takes a milestone name or its numeric ID; an unknown milestone
fails before the issue is changed.`,
		Example: `  # Edit the title
  $ copia-cli issue edit 12 --title "New title"

  # Add labels
  $ copia-cli issue edit 12 --add-label bug --add-label urgent

  # Set assignees
  $ copia-cli issue edit 12 --assignee john --assignee jane

  # Add and remove assignees
  $ copia-cli issue edit 12 --add-assignee jane --remove-assignee john

  # Set milestone by name or ID
  $ copia-cli issue edit 12 --milestone v1.0
  $ copia-cli issue edit 12 --milestone 1`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			num, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid issue number: %s", args[0])
			}
			opts.Number = num
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
			return EditRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Set title")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Set body")
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-label", nil, "Add labels by name (repository or organization labels)")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Set assignees")
	cmd.Flags().StringSliceVar(&opts.AddAssignees, "add-assignee", nil, "Add assignees by login")
	cmd.Flags().StringSliceVar(&opts.RemoveAssignees, "remove-assignee", nil, "Remove assignees by login")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Set milestone by name or ID")

	return cmd
}

func EditRun(opts *EditOptions) error {
	changesAssignees := len(opts.AddAssignees) > 0 || len(opts.RemoveAssignees) > 0
	hasIssueUpdate := opts.Title != "" || opts.Body != "" || len(opts.Assignees) > 0 || changesAssignees || opts.Milestone != ""
	hasLabelAdd := len(opts.AddLabels) > 0

	if !hasIssueUpdate && !hasLabelAdd {
		return fmt.Errorf("nothing to edit. Use --title, --body, --add-label, --assignee, --add-assignee, --remove-assignee, or --milestone")
	}
	if len(opts.Assignees) > 0 && changesAssignees {
		return fmt.Errorf("specify only one of --assignee or --add-assignee/--remove-assignee")
	}

	// Resolve the milestone first, so an unknown one fails before any change.
	var milestoneID int64
	if opts.Milestone != "" {
		id, err := shared.ResolveMilestoneID(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Milestone)
		if err != nil {
			return err
		}
		milestoneID = id
	}

	// Gitea has no endpoint that removes one assignee: compute the new list
	// from the current one and send it whole.
	assignees := opts.Assignees
	if changesAssignees {
		current, err := currentAssignees(opts)
		if err != nil {
			return err
		}
		assignees = changeAssignees(current, opts.AddAssignees, opts.RemoveAssignees)
	}

	if hasLabelAdd {
		if err := addLabels(opts); err != nil {
			return err
		}
	}

	if hasIssueUpdate {
		if err := updateIssue(opts, assignees, milestoneID); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Updated issue #%d\n", opts.Number)
	return nil
}

func updateIssue(opts *EditOptions, assignees []string, milestoneID int64) error {
	payload := map[string]interface{}{}
	if opts.Title != "" {
		payload["title"] = opts.Title
	}
	if opts.Body != "" {
		payload["body"] = opts.Body
	}
	// A non-nil empty list is sent as [] and clears the assignees.
	if assignees != nil {
		payload["assignees"] = assignees
	}
	if milestoneID > 0 {
		payload["milestone"] = milestoneID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to update issue (HTTP %d)", resp.StatusCode)
	}

	return nil
}

func addLabels(opts *EditOptions) error {
	// Gitea API expects label IDs, not names. Look up IDs first.
	labelIDs, err := shared.ResolveLabelIDs(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.AddLabels)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"labels": labelIDs,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/labels",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to add labels (HTTP %d)", resp.StatusCode)
	}

	return nil
}

func currentAssignees(opts *EditOptions) ([]string, error) {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read issue #%d (HTTP %d)", opts.Number, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var issue struct {
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
	}
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, err
	}

	logins := make([]string, 0, len(issue.Assignees))
	for _, a := range issue.Assignees {
		logins = append(logins, a.Login)
	}
	return logins, nil
}

// changeAssignees returns current without the removed logins and with the
// added ones that are not already present. The result is never nil, so
// removing the last assignee sends an empty list.
func changeAssignees(current, add, remove []string) []string {
	out := []string{}
	for _, login := range slices.Concat(current, add) {
		if !slices.Contains(remove, login) && !slices.Contains(out, login) {
			out = append(out, login)
		}
	}
	return out
}
