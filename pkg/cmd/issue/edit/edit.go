package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type EditOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Title      string
	Body       string
	AddLabels  []string
	Assignees  []string
	Milestone  int64
}

func NewCmdEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &EditOptions{}

	cmd := &cobra.Command{
		Use:   "edit <number>",
		Short: "Edit an issue",
		Long:  "Edit an issue's title, body, labels, assignees, or milestone.",
		Example: `  # Edit the title
  $ copia-cli issue edit 12 --title "New title"

  # Add labels
  $ copia-cli issue edit 12 --add-label bug --add-label urgent

  # Set assignees
  $ copia-cli issue edit 12 --assignee john --assignee jane

  # Set milestone
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
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-label", nil, "Add labels")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Set assignees")
	cmd.Flags().Int64VarP(&opts.Milestone, "milestone", "m", 0, "Set milestone ID")

	return cmd
}

func EditRun(opts *EditOptions) error {
	hasIssueUpdate := opts.Title != "" || opts.Body != "" || len(opts.Assignees) > 0 || opts.Milestone > 0
	hasLabelAdd := len(opts.AddLabels) > 0

	if !hasIssueUpdate && !hasLabelAdd {
		return fmt.Errorf("nothing to edit. Use --title, --body, --add-label, --assignee, or --milestone")
	}

	if hasLabelAdd {
		if err := addLabels(opts); err != nil {
			return err
		}
	}

	if hasIssueUpdate {
		if err := updateIssue(opts); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Updated issue #%d\n", opts.Number)
	return nil
}

func updateIssue(opts *EditOptions) error {
	payload := map[string]interface{}{}
	if opts.Title != "" {
		payload["title"] = opts.Title
	}
	if opts.Body != "" {
		payload["body"] = opts.Body
	}
	if len(opts.Assignees) > 0 {
		payload["assignees"] = opts.Assignees
	}
	if opts.Milestone > 0 {
		payload["milestone"] = opts.Milestone
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
	labelIDs, err := resolveLabelIDs(opts)
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

type labelEntry struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func decodeLabels(body []byte) ([]labelEntry, error) {
	var labels []labelEntry
	err := json.Unmarshal(body, &labels)
	return labels, err
}

func resolveLabelIDs(opts *EditOptions) ([]int64, error) {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels",
		opts.Host, opts.Owner, opts.Repo)

	// A limit of 0 reads every page, so names past the first page resolve.
	allLabels, err := cmdutil.FetchPages(opts.HTTPClient, url, opts.Token, 0, decodeLabels)
	if err != nil {
		return nil, err
	}

	nameToID := map[string]int64{}
	for _, l := range allLabels {
		nameToID[l.Name] = l.ID
	}

	var ids []int64
	for _, name := range opts.AddLabels {
		id, ok := nameToID[name]
		if !ok {
			return nil, fmt.Errorf("label %q not found in repository", name)
		}
		ids = append(ids, id)
	}

	return ids, nil
}
