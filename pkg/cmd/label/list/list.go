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

var validJSONFields = []string{"id", "name", "color", "description"}

// ListOptions holds all inputs for the label list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Limit      int
	JSON       cmdutil.JSONFlags
}

type labelEntry struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// NewCmdList creates the `copia label list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List labels in a repository",
		Long: `List labels in a Copia repository. Labels are displayed with their name, color, and description.

Use --limit to control how many labels are returned (default 30).`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli label list
  $ copia-cli label list --limit 100
  $ copia-cli label list --json name,color`,
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
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ListRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of labels")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodeLabels(body []byte) ([]labelEntry, error) {
	var labels []labelEntry
	err := json.Unmarshal(body, &labels)
	return labels, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels", opts.Host, opts.Owner, opts.Repo)
	labels, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodeLabels)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, labels)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, l := range labels {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", l.Name, l.Color, l.Description)
	}
	return w.Flush()
}
