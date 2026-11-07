package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// EditOptions holds all inputs for the repo edit command.
type EditOptions struct {
	IO           *iostreams.IOStreams
	HTTPClient   *http.Client
	Host         string
	Token        string
	Owner        string
	Repo         string
	Edits        EditRepositoryInput
	Visibility   *string
	AddTopics    []string
	RemoveTopics []string
}

// EditRepositoryInput is the body of PATCH /repos/{owner}/{repo}
// (Gitea EditRepoOption). A nil field is left out, so only the given flags
// change.
type EditRepositoryInput struct {
	Description         *string `json:"description,omitempty"`
	Website             *string `json:"website,omitempty"`
	DefaultBranch       *string `json:"default_branch,omitempty"`
	Private             *bool   `json:"private,omitempty"`
	Template            *bool   `json:"template,omitempty"`
	HasIssues           *bool   `json:"has_issues,omitempty"`
	HasWiki             *bool   `json:"has_wiki,omitempty"`
	HasProjects         *bool   `json:"has_projects,omitempty"`
	AllowMergeCommits   *bool   `json:"allow_merge_commits,omitempty"`
	AllowSquashMerge    *bool   `json:"allow_squash_merge,omitempty"`
	AllowRebase         *bool   `json:"allow_rebase,omitempty"`
	DeleteBranchOnMerge *bool   `json:"default_delete_branch_after_merge,omitempty"`
}

// NewCmdEdit creates the `copia repo edit` command.
func NewCmdEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &EditOptions{}
	var accept bool
	strs := map[string]**string{
		"description":    &opts.Edits.Description,
		"homepage":       &opts.Edits.Website,
		"default-branch": &opts.Edits.DefaultBranch,
		"visibility":     &opts.Visibility,
	}
	bools := map[string]**bool{
		"template":               &opts.Edits.Template,
		"enable-issues":          &opts.Edits.HasIssues,
		"enable-wiki":            &opts.Edits.HasWiki,
		"enable-projects":        &opts.Edits.HasProjects,
		"enable-merge-commit":    &opts.Edits.AllowMergeCommits,
		"enable-squash-merge":    &opts.Edits.AllowSquashMerge,
		"enable-rebase-merge":    &opts.Edits.AllowRebase,
		"delete-branch-on-merge": &opts.Edits.DeleteBranchOnMerge,
	}

	cmd := &cobra.Command{
		Use:   "edit [<owner/repo>]",
		Short: "Edit repository settings",
		Long: `Edit repository settings. With no argument, the repository for the current
directory is edited.

Only the given flags change. To toggle a setting off, use the --<flag>=false
syntax.

Changing repository visibility can have unexpected consequences, such as
detaching forks or exposing the repository's history. When the --visibility
flag is used, the --accept-visibility-change-consequences flag is required.`,
		Example: `  # Enable issues and wiki
  $ copia-cli repo edit --enable-issues --enable-wiki

  # Disable projects
  $ copia-cli repo edit --enable-projects=false

  # Add and remove topics
  $ copia-cli repo edit my-org/my-repo --add-topic plc,scada --remove-topic old`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if cmd.Flags().NFlag() == 0 {
				return errors.New("specify properties to edit")
			}
			if cmd.Flags().Changed("visibility") && !accept {
				return errors.New("use of --visibility flag requires --accept-visibility-change-consequences flag")
			}
			for name, p := range strs {
				if !cmd.Flags().Changed(name) {
					*p = nil
				}
			}
			for name, p := range bools {
				if !cmd.Flags().Changed(name) {
					*p = nil
				}
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			if len(args) > 0 {
				owner, repo, err := cmdutil.SplitOwnerRepo(args[0])
				if err != nil {
					return err
				}
				opts.Owner, opts.Repo = owner, repo
			} else {
				owner, repo, err := f.ResolveRepo()
				if err != nil {
					return fmt.Errorf("could not determine repository: %w", err)
				}
				opts.Owner, opts.Repo = owner, repo
			}

			opts.HTTPClient = cmdutil.NewHTTPClient()
			return EditRun(opts)
		},
	}

	opts.Edits.Description = new(string)
	opts.Edits.Website = new(string)
	opts.Edits.DefaultBranch = new(string)
	opts.Visibility = new(string)
	cmd.Flags().StringVarP(opts.Edits.Description, "description", "d", "", "Description of the repository")
	cmd.Flags().StringVar(opts.Edits.Website, "homepage", "", "Repository home page `URL`")
	cmd.Flags().StringVar(opts.Edits.DefaultBranch, "default-branch", "", "Set the default branch `name` for the repository")
	cmd.Flags().StringVar(opts.Visibility, "visibility", "", "Change the visibility of the repository to {public,private}")

	opts.Edits.Template = new(bool)
	opts.Edits.HasIssues = new(bool)
	opts.Edits.HasWiki = new(bool)
	opts.Edits.HasProjects = new(bool)
	opts.Edits.AllowMergeCommits = new(bool)
	opts.Edits.AllowSquashMerge = new(bool)
	opts.Edits.AllowRebase = new(bool)
	opts.Edits.DeleteBranchOnMerge = new(bool)
	cmd.Flags().BoolVar(opts.Edits.Template, "template", false, "Make the repository available as a template repository")
	cmd.Flags().BoolVar(opts.Edits.HasIssues, "enable-issues", false, "Enable issues in the repository")
	cmd.Flags().BoolVar(opts.Edits.HasProjects, "enable-projects", false, "Enable projects in the repository")
	cmd.Flags().BoolVar(opts.Edits.HasWiki, "enable-wiki", false, "Enable wiki in the repository")
	cmd.Flags().BoolVar(opts.Edits.AllowMergeCommits, "enable-merge-commit", false, "Enable merging pull requests via merge commit")
	cmd.Flags().BoolVar(opts.Edits.AllowSquashMerge, "enable-squash-merge", false, "Enable merging pull requests via squashed commit")
	cmd.Flags().BoolVar(opts.Edits.AllowRebase, "enable-rebase-merge", false, "Enable merging pull requests via rebase")
	cmd.Flags().BoolVar(opts.Edits.DeleteBranchOnMerge, "delete-branch-on-merge", false, "Delete head branch when pull requests are merged")

	cmd.Flags().StringSliceVar(&opts.AddTopics, "add-topic", nil, "Add repository topic")
	cmd.Flags().StringSliceVar(&opts.RemoveTopics, "remove-topic", nil, "Remove repository topic")
	cmd.Flags().BoolVar(&accept, "accept-visibility-change-consequences", false, "Accept the consequences of changing the repository visibility")

	return cmd
}

// EditRun sends the changed settings with PATCH /repos/{owner}/{repo} and,
// when --add-topic or --remove-topic change the topic list, the new list
// with PUT /repos/{owner}/{repo}/topics.
func EditRun(opts *EditOptions) error {
	if opts.Visibility != nil {
		switch *opts.Visibility {
		case "public", "private":
			private := *opts.Visibility == "private"
			opts.Edits.Private = &private
		default:
			return fmt.Errorf("invalid visibility %q: valid values are {public|private}", *opts.Visibility)
		}
	}

	repoURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", opts.Host, opts.Owner, opts.Repo)

	if opts.Edits != (EditRepositoryInput{}) {
		err := send(opts, "PATCH", repoURL, opts.Edits, http.StatusOK, "failed to edit repository")
		// Gitea answers 403 when its FORCE_PRIVATE setting keeps a
		// repository from becoming public; app.copia.io does so.
		var status statusError
		if errors.As(err, &status) && status.code == http.StatusForbidden && opts.Edits.Private != nil && !*opts.Edits.Private {
			return fmt.Errorf("%w: the server may not allow public repositories", err)
		}
		if err != nil {
			return err
		}
	}

	if len(opts.AddTopics) > 0 || len(opts.RemoveTopics) > 0 {
		if err := editTopics(opts, repoURL+"/topics"); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Edited repository %s/%s\n", opts.Owner, opts.Repo)
	return nil
}

// editTopics reads the current topics, adds and removes the given ones and
// writes the list back only when it changed. Copia stores topics in lower
// case without surrounding space, so the given ones are compared that way.
func editTopics(opts *EditOptions, topicsURL string) error {
	req, err := http.NewRequest("GET", topicsURL+"?limit=50", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get topics (HTTP %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading topics: %w", err)
	}
	var current struct {
		Topics []string `json:"topics"`
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("parsing topics: %w", err)
	}

	topics := slices.Clone(current.Topics)
	for _, t := range opts.AddTopics {
		t = normalizeTopic(t)
		if t != "" && !slices.Contains(topics, t) {
			topics = append(topics, t)
		}
	}
	topics = slices.DeleteFunc(topics, func(t string) bool {
		return slices.ContainsFunc(opts.RemoveTopics, func(r string) bool { return normalizeTopic(r) == t })
	})
	if topics == nil {
		topics = []string{}
	}
	if slices.Equal(topics, current.Topics) {
		return nil
	}

	payload := struct {
		Topics []string `json:"topics"`
	}{topics}
	return send(opts, "PUT", topicsURL, payload, http.StatusNoContent, "failed to set topics")
}

func normalizeTopic(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}

// send sends payload as JSON and fails with failMsg and the status unless
// the API answers want.
func send(opts *EditOptions, method, url string, payload any, want int, failMsg string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
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
	if resp.StatusCode != want {
		return statusError{msg: failMsg, code: resp.StatusCode}
	}
	return nil
}

// statusError is an unexpected API status.
type statusError struct {
	msg  string
	code int
}

func (e statusError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", e.msg, e.code)
}
