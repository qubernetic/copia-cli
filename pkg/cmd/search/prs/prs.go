package prs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	issueshared "github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"number", "title", "state", "html_url", "repository", "user", "labels", "created_at", "updated_at"}

// SearchOptions holds all inputs for the search prs command.
type SearchOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Query      string
	State      string
	Labels     []string
	Limit      int
	Author     string
	Assignee   string
	Owner      string
	Repo       string // owner/repo from -R, matched on each result
	JSON       cmdutil.JSONFlags
	Web        bool
}

type nameRef struct {
	Name string `json:"name"`
}

type userRef struct {
	Login string `json:"login"`
}

type repoRef struct {
	FullName string `json:"full_name"`
}

type prEntry struct {
	Number      int64     `json:"number"`
	Title       string    `json:"title"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	Repository  repoRef   `json:"repository"`
	User        userRef   `json:"user"`
	Labels      []nameRef `json:"labels"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	PullRequest *struct {
		Merged bool `json:"merged"`
	} `json:"pull_request,omitempty"`
}

// NewCmdSearchPRs creates the `copia search prs` command.
func NewCmdSearchPRs(f *cmdutil.Factory) *cobra.Command {
	opts := &SearchOptions{}

	cmd := &cobra.Command{
		Use:   "prs [<query>]",
		Short: "Search for pull requests",
		Long: `Search pull requests across every repository the token can read, with
GET /repos/issues/search. By default all states are searched; results come
newest first.

--author takes a login, "@me" is the authenticated user; the author is also
checked on each result, since app.copia.io ignores the created_by filter. The search endpoint
filters assignees only on the authenticated user, so --assignee takes "@me"
or your own login. --owner limits the search to the repositories of a user
or organization; -R owner/repo to one repository. Labels match any of the
given names.

Use --web to open the search in the browser instead; no API call is made, so
no token is needed. Copia has no web search across repositories, so --web
opens the pull request dashboard (/pulls, or /org/<owner>/pulls with --owner,
which needs membership of that organization) with the query, --state,
--author and --assignee; the dashboard shows open pull requests unless
--state closed is given. With -R it opens the pull request list of that
repository, which applies --author and --assignee @me only. --label is not
applied in the browser, since the web pages take labels only by ID.`,
		Example: `  $ copia-cli search prs timeout
  $ copia-cli search prs --author @me --state open
  $ copia-cli search prs sensor --owner my-org --label bug
  $ copia-cli search prs fix -R my-org/plc --json number,title,state
  $ copia-cli search prs --author @me --web`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 && f.Repo == "" {
				return errors.New("specify search keywords or flags")
			}
			if len(args) == 1 {
				opts.Query = args[0]
			}
			opts.IO = f.IOStreams
			opts.Repo = f.Repo
			if opts.Web {
				return openSearch(f, opts)
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return SearchRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.State, "state", "s", "", "Filter by state: {open|closed}")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by label")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of results")
	cmd.Flags().StringVar(&opts.Author, "author", "", "Filter by author")
	cmd.Flags().StringVar(&opts.Assignee, "assignee", "", "Filter by assignee (@me)")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "Filter on repository owner")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the search query in the web browser")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

// openSearch opens the search in the browser: the pull request list of the
// -R repository, or else the pull request dashboard of the user or of the
// --owner organization. It makes no API call, so no token is needed.
func openSearch(f *cmdutil.Factory, opts *SearchOptions) error {
	state, err := searchState(opts.State)
	if err != nil {
		return err
	}
	owner, err := searchOwner(opts)
	if err != nil {
		return err
	}
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}

	q := url.Values{}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	var ignored []string
	if len(opts.Labels) > 0 {
		ignored = append(ignored, "label")
	}

	// Both web pages read type=created_by and type=assigned for the
	// authenticated user, and poster as a login. The dashboard also reads
	// assignee as a login; the repository list takes it only by ID.
	if opts.Author == "@me" {
		q.Set("type", "created_by")
	} else if opts.Author != "" {
		q.Set("poster", opts.Author)
	}
	switch {
	case opts.Assignee == "":
	case opts.Assignee == "@me" && !q.Has("type"):
		q.Set("type", "assigned")
	case opts.Assignee != "@me" && opts.Repo == "":
		q.Set("assignee", opts.Assignee)
	default:
		ignored = append(ignored, "assignee")
	}
	cmdutil.WarnWebIgnored(opts.IO, ignored...)

	var u string
	switch {
	case opts.Repo != "":
		repoOwner, repo, err := cmdutil.SplitOwnerRepo(opts.Repo)
		if err != nil {
			return err
		}
		q.Set("state", state)
		u = cmdutil.RepoWebURL(host, repoOwner, repo, "pulls")
	case owner != "":
		u = fmt.Sprintf("https://%s/org/%s/pulls", host, url.PathEscape(owner))
	default:
		u = fmt.Sprintf("https://%s/pulls", host)
	}
	// The dashboard knows open and closed only, open when state is not set.
	if opts.Repo == "" && opts.State != "" {
		q.Set("state", opts.State)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return cmdutil.OpenInBrowser(opts.IO, f.Browser, u)
}

// searchState checks --state and returns it, or "all" when it is empty.
func searchState(state string) (string, error) {
	switch state {
	case "":
		return "all", nil
	case "open", "closed":
		return state, nil
	default:
		return "", fmt.Errorf("invalid state %q: use open or closed", state)
	}
}

// searchOwner returns the owner to search in: --owner, or the owner of the
// -R repository, which --owner must then match.
func searchOwner(opts *SearchOptions) (string, error) {
	owner := opts.Owner
	if opts.Repo != "" {
		repoOwner, _, err := cmdutil.SplitOwnerRepo(opts.Repo)
		if err != nil {
			return "", err
		}
		if owner != "" && !strings.EqualFold(owner, repoOwner) {
			return "", fmt.Errorf("--owner %q does not match the owner of --repo %q", owner, opts.Repo)
		}
		owner = repoOwner
	}
	return owner, nil
}

func decodePRs(body []byte) ([]prEntry, error) {
	var prs []prEntry
	err := json.Unmarshal(body, &prs)
	return prs, err
}

// SearchRun runs the search and prints the pull requests found.
func SearchRun(opts *SearchOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}
	state, err := searchState(opts.State)
	if err != nil {
		return err
	}
	owner, err := searchOwner(opts)
	if err != nil {
		return err
	}

	q := url.Values{}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	q.Set("type", "pulls")
	q.Set("state", state)
	if len(opts.Labels) > 0 {
		q.Set("labels", strings.Join(opts.Labels, ","))
	}
	var me string
	login := func() (string, error) {
		if me == "" {
			logins, err := issueshared.ResolveAssignees(opts.HTTPClient, opts.Host, opts.Token, []string{"@me"})
			if err != nil {
				return "", err
			}
			me = logins[0]
		}
		return me, nil
	}
	author := opts.Author
	switch author {
	case "":
	case "@me":
		q.Set("created", "true")
		var err error
		if author, err = login(); err != nil {
			return err
		}
	default:
		q.Set("created_by", author)
	}
	if opts.Assignee != "" {
		if opts.Assignee != "@me" {
			if _, err := login(); err != nil {
				return err
			}
			if !strings.EqualFold(me, opts.Assignee) {
				return fmt.Errorf("--assignee %q: the search endpoint filters only on the authenticated user (%q), use @me", opts.Assignee, me)
			}
		}
		q.Set("assigned", "true")
	}
	if owner != "" {
		q.Set("owner", owner)
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/issues/search?%s", opts.Host, q.Encode())
	var prs []prEntry
	// app.copia.io ignores created_by, and the endpoint has no repository
	// filter, so both are also checked on each result.
	if opts.Repo != "" || author != "" {
		prs, err = cmdutil.FetchFilteredPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodePRs, func(pr prEntry) bool {
			return (opts.Repo == "" || strings.EqualFold(pr.Repository.FullName, opts.Repo)) &&
				(author == "" || strings.EqualFold(pr.User.Login, author))
		})
	} else {
		prs, err = cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodePRs)
	}
	if err != nil {
		return searchError(err, opts.Author, owner)
	}

	for i := range prs {
		if prs[i].PullRequest != nil && prs[i].PullRequest.Merged {
			prs[i].State = "merged"
		}
		prs[i].PullRequest = nil
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, prs)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, pr := range prs {
		_, _ = fmt.Fprintf(w, "%s\t#%d\t%s\t%s\n", pr.Repository.FullName, pr.Number, pr.Title, pr.State)
	}
	return w.Flush()
}

// searchError names the filter Gitea refused: HTTP 404 for an unknown
// created_by user, HTTP 400 for an unknown owner.
func searchError(err error, author, owner string) error {
	var httpErr *cmdutil.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusNotFound && author != "" && author != "@me":
			return fmt.Errorf("no user named %q", author)
		case httpErr.StatusCode == http.StatusBadRequest && owner != "":
			return fmt.Errorf("no user or organization named %q", owner)
		}
	}
	return err
}
