package list

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"number", "title", "state", "user", "base", "head", "updated_at"}

// ListOptions holds all inputs for the pr list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	State      string
	Limit      int
	Web        bool
	JSON       cmdutil.JSONFlags
}

type branchRef struct {
	Label string `json:"label"`
}

type userRef struct {
	Login string `json:"login"`
}

type prEntry struct {
	Number    int64     `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	User      userRef   `json:"user"`
	Base      branchRef `json:"base"`
	Head      branchRef `json:"head"`
	UpdatedAt string    `json:"updated_at"`
}

// NewCmdList creates the `copia pr list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List pull requests",
		Long: `List pull requests in a Copia repository. By default, only open pull requests are listed.

Use --state to filter by open, closed, merged, or all states.
Use --limit to control how many pull requests are returned (default 30).

Use --web to open the pull request list in the browser instead; no API call is
made, so no token is needed. The browser list has no merged filter, so
--state merged shows the closed pull requests there.`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli pr list
  $ copia-cli pr list --state closed
  $ copia-cli pr list --state closed --web
  $ copia-cli pr list --json number,title,state`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if opts.Web {
				return openList(f, opts)
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
			return ListRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: {open|closed|all}")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "List pull requests in the web browser")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodePRs(body []byte) ([]prEntry, error) {
	var prs []prEntry
	err := json.Unmarshal(body, &prs)
	return prs, err
}

func validateState(state string) error {
	switch state {
	case "open", "closed", "merged", "all":
		return nil
	}
	return fmt.Errorf("invalid state %q: valid values are {open|closed|merged|all}", state)
}

// openList opens the pull request list of the repository in the browser with
// the state filter of the Gitea web list, which knows open, closed and all.
// It has no merged filter, so --state merged opens the closed list with a
// warning. No API call is made.
func openList(f *cmdutil.Factory, opts *ListOptions) error {
	if err := validateState(opts.State); err != nil {
		return err
	}
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}
	owner, repo, err := f.ResolveRepo()
	if err != nil {
		return err
	}

	state := opts.State
	if state == "merged" {
		state = "closed"
		_, _ = fmt.Fprintln(opts.IO.ErrOut, "warning: --state merged is not applied in the browser, showing closed pull requests")
	}
	q := url.Values{}
	q.Set("state", state)
	return cmdutil.OpenInBrowser(opts.IO, f.Browser, cmdutil.RepoWebURL(host, owner, repo, "pulls")+"?"+q.Encode())
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	if err := validateState(opts.State); err != nil {
		return err
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls?state=%s",
		opts.Host, opts.Owner, opts.Repo, opts.State)

	prs, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodePRs)
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, prs)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, pr := range prs {
		_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\t%s\t%s <- %s\n",
			pr.Number, pr.Title, pr.State, pr.User.Login, pr.Base.Label, pr.Head.Label)
	}
	return w.Flush()
}
