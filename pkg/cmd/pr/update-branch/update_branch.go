package updatebranch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/pr/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// UpdateBranchOptions holds all inputs for the pr update-branch command.
type UpdateBranchOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Rebase     bool
}

// NewCmdUpdateBranch creates the `copia pr update-branch` command.
func NewCmdUpdateBranch(f *cmdutil.Factory) *cobra.Command {
	opts := &UpdateBranchOptions{}

	cmd := &cobra.Command{
		Use:   "update-branch {<number> | <url>}",
		Short: "Update a pull request branch",
		Long: "Update a pull request branch with the latest changes of the base branch, by number or URL.\n\n" +
			"The base branch is merged into the head branch with a merge commit. " +
			"With --rebase the head branch is rebased onto the base branch instead.\n\n" +
			"A branch that is already up to date is left as it is. " +
			"A closed or merged pull request cannot be updated.",
		Example: `  # Merge the base branch into the branch of pull request 7
  $ copia-cli pr update-branch 7

  # Rebase the branch onto the base branch
  $ copia-cli pr update-branch https://app.copia.io/my-org/my-repo/pulls/7 --rebase`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := shared.ParsePRArg(args[0])
			if err != nil {
				return err
			}
			opts.Number = ref.Number
			opts.IO = f.IOStreams

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
			return UpdateBranchRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Rebase, "rebase", false, "Update the pull request branch using the rebase strategy")

	return cmd
}

type branchRef struct {
	Ref  string `json:"ref"`
	Repo struct {
		FullName string `json:"full_name"`
	} `json:"repo"`
}

type prState struct {
	Title  string    `json:"title"`
	State  string    `json:"state"`
	Merged bool      `json:"merged"`
	Head   branchRef `json:"head"`
	Base   branchRef `json:"base"`
}

// UpdateBranchRun updates the head branch with
// POST /repos/{owner}/{repo}/pulls/{index}/update?style=merge|rebase.
func UpdateBranchRun(opts *UpdateBranchOptions) error {
	repoURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", opts.Host, opts.Owner, opts.Repo)
	prURL := fmt.Sprintf("%s/pulls/%d", repoURL, opts.Number)
	ref := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)

	pr, err := fetchPR(opts, prURL, ref)
	if err != nil {
		return err
	}
	if pr.Merged {
		return fmt.Errorf("pull request %s (%s) can't be updated because it was already merged", ref, pr.Title)
	}
	if pr.State != "open" {
		return fmt.Errorf("pull request %s (%s) can't be updated because it is closed", ref, pr.Title)
	}
	// Gitea answers an update of an up-to-date branch with HTTP 500, so the
	// command asks first, as gh does.
	if upToDate(opts, repoURL, pr) {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! The branch of pull request %s (%s) is already up to date\n", ref, pr.Title)
		return nil
	}

	style := "merge"
	if opts.Rebase {
		style = "rebase"
	}
	resp, err := send(opts, "POST", prURL+"/update?style="+style)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Message string `json:"message"`
		}
		if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return fmt.Errorf("failed to update the branch of pull request %s (HTTP %d): %s", ref, resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("failed to update the branch of pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Updated the branch of pull request %s (%s)\n", ref, pr.Title)
	return nil
}

// upToDate reports whether the base branch has no commit that the head branch
// lacks, with GET /repos/{owner}/{repo}/compare/{head}...{base}. It answers
// false when it cannot tell (head branch in a fork, a failed compare), so the
// update is sent and the server decides.
func upToDate(opts *UpdateBranchOptions, repoURL string, pr *prState) bool {
	if pr.Head.Repo.FullName != pr.Base.Repo.FullName {
		return false
	}
	resp, err := send(opts, "GET", repoURL+"/compare/"+escapeBranch(pr.Head.Ref)+"..."+escapeBranch(pr.Base.Ref))
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var cmp struct {
		TotalCommits int `json:"total_commits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cmp); err != nil {
		return false
	}
	return cmp.TotalCommits == 0
}

// escapeBranch escapes each segment of a branch name for a URL path and keeps
// the slashes, which the compare route takes as they are.
func escapeBranch(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// fetchPR reads the pull request with GET /repos/{owner}/{repo}/pulls/{index}.
func fetchPR(opts *UpdateBranchOptions, prURL, ref string) (*prState, error) {
	resp, err := send(opts, "GET", prURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("pull request %s not found", ref)
	default:
		return nil, fmt.Errorf("failed to read pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pr prState
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func send(opts *UpdateBranchOptions, method, target string) (*http.Response, error) {
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	return opts.HTTPClient.Do(req)
}
