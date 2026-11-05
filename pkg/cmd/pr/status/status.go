package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	issueshared "github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// sectionSize is how many pull requests a section shows, as in gh.
const sectionSize = 10

// titleWidth is the width gh truncates a title to.
const titleWidth = 50

var validJSONFields = []string{"number", "title", "state", "html_url", "user", "head", "base", "updated_at", "mergeable", "draft"}

// StatusOptions holds all inputs for the pr status command.
type StatusOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	// HasRepoOverride is set by -R: the current branch section is left out,
	// since the local branch belongs to another repository.
	HasRepoOverride bool
	ConflictStatus  bool
	JSON            cmdutil.JSONFlags
	// Branch returns the checked-out branch; an error means there is none.
	Branch func() (string, error)
}

type userRef struct {
	Login string `json:"login"`
}

type branchRef struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
}

type prEntry struct {
	Number    int64     `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	User      userRef   `json:"user"`
	Head      branchRef `json:"head"`
	Base      branchRef `json:"base"`
	UpdatedAt string    `json:"updated_at"`
	Mergeable bool      `json:"mergeable"`
	Draft     bool      `json:"draft"`

	defaultBranch string
}

// apiPR is a pull request as Gitea returns it.
type apiPR struct {
	prEntry
	Merged bool `json:"merged"`
	Base   struct {
		branchRef
		Repo struct {
			DefaultBranch string `json:"default_branch"`
		} `json:"repo"`
	} `json:"base"`
}

// searchHit is one result of the issue search.
type searchHit struct {
	Number     int64 `json:"number"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// NewCmdStatus creates the `copia pr status` command.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of relevant pull requests",
		Long: `Show the pull requests of the repository that are relevant to you: the one
of the current branch, the open ones you created and the open ones requesting
a review from you. Each list shows the ten most recently updated pull
requests. With -R the current branch is left out.

With --json, the output is one object with the keys currentBranch (a pull
request or null), createdBy and needsReview, each with the requested fields.`,
		Example: `  $ copia-cli pr status
  $ copia-cli pr status --conflict-status
  $ copia-cli pr status --json number,title --jq '.needsReview[].number'`,
		Args: cobra.NoArgs,
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
			opts.HasRepoOverride = f.Repo != ""
			opts.HTTPClient = cmdutil.NewHTTPClient()
			opts.Branch = currentBranch
			return StatusRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ConflictStatus, "conflict-status", "c", false, "Display the merge conflict status of each pull request")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

// currentBranch returns the checked-out branch of the git repository.
func currentBranch() (string, error) {
	out, err := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func decodePRs(body []byte) ([]prEntry, error) {
	var raw []apiPR
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	prs := make([]prEntry, 0, len(raw))
	for _, r := range raw {
		p := r.prEntry
		p.Base = r.Base.branchRef
		p.defaultBranch = r.Base.Repo.DefaultBranch
		if r.Merged {
			p.State = "merged"
		}
		prs = append(prs, p)
	}
	return prs, nil
}

func decodeHits(body []byte) ([]searchHit, error) {
	var hits []searchHit
	err := json.Unmarshal(body, &hits)
	return hits, err
}

// StatusRun fetches the three sections and prints them. One listing of the
// open pull requests gives the pull request of the current branch and the
// ones you created; the closed ones are read only when the branch has no
// open pull request. The review requests come from the issue search, which
// spans every repository, so its results are matched to this repository.
func StatusRun(opts *StatusOptions) error {
	users, err := issueshared.ResolveAssignees(opts.HTTPClient, opts.Host, opts.Token, []string{"@me"})
	if err != nil {
		return err
	}
	me := users[0]

	pullsURL := func(state string) string {
		return fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls?", opts.Host, opts.Owner, opts.Repo) +
			url.Values{"state": {state}, "sort": {"recentupdate"}}.Encode()
	}
	open, err := cmdutil.FetchPages(opts.HTTPClient, pullsURL("open"), opts.Token, 0, decodePRs)
	if err != nil {
		return err
	}
	sortByUpdated(open)

	branch := ""
	var current *prEntry
	if !opts.HasRepoOverride {
		branch, _ = opts.Branch()
	}
	if branch != "" {
		onBranch := func(p prEntry) bool { return p.Head.Ref == branch }
		if i := slices.IndexFunc(open, onBranch); i >= 0 {
			current = &open[i]
		} else {
			closed, err := cmdutil.FetchFilteredPages(opts.HTTPClient, pullsURL("closed"), opts.Token, 1, decodePRs, onBranch)
			if err != nil {
				return err
			}
			// gh leaves out a closed pull request of the default branch.
			if len(closed) > 0 && closed[0].defaultBranch != branch {
				current = &closed[0]
			}
		}
	}

	searchURL := fmt.Sprintf("https://%s/api/v1/repos/issues/search?", opts.Host) +
		url.Values{"type": {"pulls"}, "state": {"open"}, "review_requested": {"true"}, "owner": {opts.Owner}}.Encode()
	fullName := opts.Owner + "/" + opts.Repo
	hits, err := cmdutil.FetchFilteredPages(opts.HTTPClient, searchURL, opts.Token, 0, decodeHits, func(h searchHit) bool {
		return strings.EqualFold(h.Repository.FullName, fullName)
	})
	if err != nil {
		return err
	}

	createdBy := []prEntry{}
	needsReview := []prEntry{}
	for _, p := range open {
		if p.User.Login == me {
			createdBy = append(createdBy, p)
		}
		if slices.ContainsFunc(hits, func(h searchHit) bool { return h.Number == p.Number }) {
			needsReview = append(needsReview, p)
		}
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.JSON.IsJSON() {
		return opts.JSON.PrintSections(opts.IO.Out, map[string]any{
			"currentBranch": current,
			"createdBy":     createdBy[:min(len(createdBy), sectionSize)],
			"needsReview":   needsReview[:min(len(needsReview), sectionSize)],
		})
	}

	out := opts.IO.Out
	_, _ = fmt.Fprintf(out, "\nRelevant pull requests in %s\n\n", fullName)

	if !opts.HasRepoOverride {
		_, _ = fmt.Fprintln(out, "Current branch")
		switch {
		case current != nil:
			printPRs(opts, []prEntry{*current})
		case branch == "":
			_, _ = fmt.Fprintln(out, "  There is no current branch")
		default:
			_, _ = fmt.Fprintf(out, "  There is no pull request associated with [%s]\n", branch)
		}
		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintln(out, "Created by you")
	if len(createdBy) > 0 {
		printPRs(opts, createdBy)
	} else {
		_, _ = fmt.Fprintln(out, "  You have no open pull requests")
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Requesting a code review from you")
	if len(needsReview) > 0 {
		printPRs(opts, needsReview)
	} else {
		_, _ = fmt.Fprintln(out, "  You have no pull requests to review")
	}
	_, _ = fmt.Fprintln(out)
	return nil
}

// sortByUpdated orders pull requests by their last update, newest first,
// in case the server does not apply sort=recentupdate.
func sortByUpdated(prs []prEntry) {
	sort.SliceStable(prs, func(a, b int) bool {
		ta, _ := time.Parse(time.RFC3339, prs[a].UpdatedAt)
		tb, _ := time.Parse(time.RFC3339, prs[b].UpdatedAt)
		return ta.After(tb)
	})
}

// printPRs prints the first sectionSize pull requests as gh does: number,
// title and head branch, the state of a closed or merged one and, with
// --conflict-status, whether an open one merges cleanly.
func printPRs(opts *StatusOptions, prs []prEntry) {
	out := opts.IO.Out
	for _, p := range prs[:min(len(prs), sectionSize)] {
		head := p.Head.Label
		if head == "" {
			head = p.Head.Ref
		}
		_, _ = fmt.Fprintf(out, "  #%d  %s [%s]", p.Number, truncate(p.Title), head)
		switch {
		case p.State != "open" && p.State != "":
			_, _ = fmt.Fprintf(out, " - %s", strings.ToUpper(p.State[:1])+p.State[1:])
		case opts.ConflictStatus && p.Mergeable:
			_, _ = fmt.Fprint(out, " ✓ No merge conflicts")
		case opts.ConflictStatus:
			_, _ = fmt.Fprint(out, " × Merge conflicts")
		}
		_, _ = fmt.Fprintln(out)
	}
	if more := len(prs) - sectionSize; more > 0 {
		_, _ = fmt.Fprintf(out, "  And %d more\n", more)
	}
}

// truncate shortens a title to titleWidth runes with "...", as gh does.
func truncate(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	r := []rune(title)
	if len(r) <= titleWidth {
		return title
	}
	return string(r[:titleWidth-3]) + "..."
}
