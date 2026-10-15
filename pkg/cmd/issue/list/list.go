package list

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"number", "title", "state", "labels", "updated_at"}

// ListOptions holds all inputs for the issue list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	State      string
	Limit      int
	Labels     []string
	JSON       cmdutil.JSONFlags
}

type labelRef struct {
	Name string `json:"name"`
}

type issueEntry struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	UpdatedAt string     `json:"updated_at"`
	Labels    []labelRef `json:"labels"`
}

// NewCmdList creates the `copia issue list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List issues in a repository",
		Long: `List issues in a Copia repository. By default, only open issues are listed.

Use --state to filter by open, closed, or all states.
Use --label to filter by one or more label names.
Use --limit to control how many issues are returned (default 30).`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli issue list
  $ copia-cli issue list --state closed
  $ copia-cli issue list --json number,title,state`,
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
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of issues")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by label")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodeIssues(body []byte) ([]issueEntry, error) {
	var issues []issueEntry
	err := json.Unmarshal(body, &issues)
	return issues, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	switch opts.State {
	case "open", "closed", "all":
	default:
		return fmt.Errorf("invalid state %q: valid values are {open|closed|all}", opts.State)
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues?state=%s&type=issues",
		opts.Host, opts.Owner, opts.Repo, opts.State)
	if len(opts.Labels) > 0 {
		u += "&labels=" + strings.Join(opts.Labels, ",")
	}

	issues, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodeIssues)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return cmdutil.PrintJSON(opts.IO.Out, issues)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, i := range issues {
		labels := ""
		for j, l := range i.Labels {
			if j > 0 {
				labels += ", "
			}
			labels += l.Name
		}
		_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\t%s\n", i.Number, i.Title, i.State, labels)
	}
	return w.Flush()
}
