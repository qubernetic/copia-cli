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

type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Limit      int
	JSON       cmdutil.JSONFlags
}

type orgEntry struct {
	Username    string `json:"username"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
}

func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List your organizations",
		Long: `List organizations that the authenticated user belongs to.

Use --limit to control how many organizations are returned (default 30).`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli org list
  $ copia-cli org list --limit 100
  $ copia-cli org list --json username,full_name`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ListRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of organizations")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, []string{"username", "full_name", "description"})
	return cmd
}

func decodeOrgs(body []byte) ([]orgEntry, error) {
	var orgs []orgEntry
	err := json.Unmarshal(body, &orgs)
	return orgs, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	u := fmt.Sprintf("https://%s/api/v1/user/orgs", opts.Host)
	orgs, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodeOrgs)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, orgs)
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, o := range orgs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", o.Username, o.FullName, o.Description)
	}
	return w.Flush()
}
