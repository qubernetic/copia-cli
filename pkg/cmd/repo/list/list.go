package list

import (
	"encoding/json"
	"fmt"
	"net/http"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"full_name", "description", "private", "updated_at"}

// ListOptions holds all inputs for the repo list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Org        string
	Limit      int
	Visibility string
	JSON       cmdutil.JSONFlags
}

type repoEntry struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	UpdatedAt   string `json:"updated_at"`
}

// NewCmdList creates the `copia repo list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List repositories",
		Long: `List repositories owned by the authenticated user. Use --org to list
repositories for a specific organization instead.`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli repo list
  $ copia-cli repo list --org my-org
  $ copia-cli repo list --visibility private
  $ copia-cli repo list --json full_name,description`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = &http.Client{}
			return ListRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "List repositories for an organization")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of repositories")
	cmd.Flags().StringVar(&opts.Visibility, "visibility", "", "Filter by visibility: {public|private}")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodeRepos(body []byte) ([]repoEntry, error) {
	var repos []repoEntry
	err := json.Unmarshal(body, &repos)
	return repos, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	switch opts.Visibility {
	case "", "public", "private":
	default:
		return fmt.Errorf("invalid visibility %q: valid values are {public|private}", opts.Visibility)
	}

	endpoint :="/api/v1/user/repos"
	if opts.Org != "" {
		endpoint = fmt.Sprintf("/api/v1/orgs/%s/repos", opts.Org)
	}

	var repos []repoEntry
	var err error
	if opts.Visibility == "" {
		repos, err = cmdutil.FetchPages(opts.HTTPClient, "https://"+opts.Host+endpoint, opts.Token, opts.Limit, decodeRepos)
	} else {
		// The list endpoints take no visibility filter; drop the others per page.
		private := opts.Visibility == "private"
		repos, err = cmdutil.FetchFilteredPages(opts.HTTPClient, "https://"+opts.Host+endpoint, opts.Token, opts.Limit, decodeRepos,
			func(r repoEntry) bool { return r.Private == private })
	}
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, repos)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, r := range repos {
		visibility := "public"
		if r.Private {
			visibility = "private"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", r.FullName, r.Description, visibility)
	}
	return w.Flush()
}
