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
Use --limit to control how many pull requests are returned (default 30).`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli pr list
  $ copia-cli pr list --state closed
  $ copia-cli pr list --json number,title,state`,
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
			opts.HTTPClient = &http.Client{}
			return ListRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: {open|closed|all}")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodePRs(body []byte) ([]prEntry, error) {
	var prs []prEntry
	err := json.Unmarshal(body, &prs)
	return prs, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	switch opts.State {
	case "open", "closed", "merged", "all":
	default:
		return fmt.Errorf("invalid state %q: valid values are {open|closed|merged|all}", opts.State)
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls?state=%s",
		opts.Host, opts.Owner, opts.Repo, opts.State)

	prs, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodePRs)
	if err != nil {
		return err
	}

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
