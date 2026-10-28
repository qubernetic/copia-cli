package repos

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

var validJSONFields = []string{"full_name", "description", "html_url"}

type SearchOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Query      string
	Sort       string
	Order      string
	Limit      int
	JSON       cmdutil.JSONFlags
}

type repoEntry struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
}

type searchResponse struct {
	Data []repoEntry `json:"data"`
}

func NewCmdSearchRepos(f *cmdutil.Factory) *cobra.Command {
	opts := &SearchOptions{}

	cmd := &cobra.Command{
		Use:   "repos <query>",
		Short: "Search repositories",
		Long:  "Search for repositories on Copia matching the given query string.",
		Example: `  $ copia-cli search repos plc
  $ copia-cli search repos plc --sort stars
  $ copia-cli search repos "automation controller" --json fullName,description`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Query = args[0]
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = &http.Client{}
			return SearchRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Sort, "sort", "", "Sort fetched repositories: {alpha|created|updated|size|stars|forks}")
	cmd.Flags().StringVar(&opts.Order, "order", "desc", "Order of repositories returned, ignored unless '--sort' flag is specified: {asc|desc}")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of results")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

// decodeSearch unwraps the repositories from one page of search results.
func decodeSearch(body []byte) ([]repoEntry, error) {
	var result searchResponse
	err := json.Unmarshal(body, &result)
	return result.Data, err
}

func SearchRun(opts *SearchOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	switch opts.Sort {
	case "", "alpha", "created", "updated", "size", "stars", "forks":
	default:
		return fmt.Errorf("invalid sort %q: valid values are {alpha|created|updated|size|stars|forks}", opts.Sort)
	}

	order := opts.Order
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		return fmt.Errorf("invalid order %q: valid values are {asc|desc}", opts.Order)
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/search?q=%s",
		opts.Host, url.QueryEscape(opts.Query))
	if opts.Sort != "" {
		u += "&sort=" + opts.Sort + "&order=" + order
	}

	repos, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodeSearch)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return cmdutil.PrintJSON(opts.IO.Out, repos)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, r := range repos {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", r.FullName, r.Description)
	}
	return w.Flush()
}
