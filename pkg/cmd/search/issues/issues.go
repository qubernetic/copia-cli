package issues

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// SearchOptions holds all inputs for the search issues command.
type SearchOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Query      string
	State      string
	Labels     []string
	Limit      int
	JSON       cmdutil.JSONFlags
	Web        bool
}

type issueEntry struct {
	Number int64  `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

// NewCmdSearchIssues creates the `copia search issues` command.
func NewCmdSearchIssues(f *cmdutil.Factory) *cobra.Command {
	opts := &SearchOptions{}

	cmd := &cobra.Command{
		Use:   "issues <query>",
		Short: "Search issues in a repository",
		Long: `Search issues within a repository by keyword. By default, searches across
all states (open and closed). Use --state and --label to filter. Results
come newest first. Requires repo context via -R flag or git remote.

Use --web to open the issue list of the repository in the browser with the
query and --state instead; no API call is made, so no token is needed. The
web list takes labels only by ID, so --label is not applied there.`,
		Example: `  $ copia-cli search issues "sensor timeout"
  $ copia-cli search issues bug --state closed
  $ copia-cli search issues timeout --label bug --label plc
  $ copia-cli search issues timeout --state open --web`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Query = args[0]
			opts.IO = f.IOStreams
			if opts.Web {
				return openSearch(f, opts)
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
			return SearchRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.State, "state", "s", "", "Filter by state: {open|closed}")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by label")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of results")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the search query in the web browser")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, []string{"number", "title", "state"})

	return cmd
}

// openSearch opens the issue list of the repository with the query and the
// state in the browser. It makes no API call, so no token is needed.
func openSearch(f *cmdutil.Factory, opts *SearchOptions) error {
	host, err := f.ResolveHost()
	if err != nil {
		return err
	}
	owner, repo, err := f.ResolveRepo()
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("q", opts.Query)
	q.Set("state", searchState(opts.State))
	if len(opts.Labels) > 0 {
		cmdutil.WarnWebIgnored(opts.IO, "label")
	}
	return cmdutil.OpenInBrowser(opts.IO, f.Browser, cmdutil.RepoWebURL(host, owner, repo, "issues")+"?"+q.Encode())
}

// searchState returns state, or "all" when it is empty.
func searchState(state string) string {
	if state == "" {
		return "all"
	}
	return state
}

func decodeIssues(body []byte) ([]issueEntry, error) {
	var issues []issueEntry
	err := json.Unmarshal(body, &issues)
	return issues, err
}

func SearchRun(opts *SearchOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	state := searchState(opts.State)

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues?q=%s&type=issues&state=%s",
		opts.Host, opts.Owner, opts.Repo, url.QueryEscape(opts.Query), url.QueryEscape(state))
	if len(opts.Labels) > 0 {
		u += "&labels=" + strings.Join(opts.Labels, ",")
	}

	issues, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodeIssues)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, issues)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, i := range issues {
		_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\n", i.Number, i.Title, i.State)
	}
	return w.Flush()
}
