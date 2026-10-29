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
	All        bool
	Limit      int
	JSON       cmdutil.JSONFlags
}

type subject struct {
	Title string `json:"title"`
	Type  string `json:"type"`
}

type repoRef struct {
	FullName string `json:"full_name"`
}

type notification struct {
	ID         int64   `json:"id"`
	Subject    subject `json:"subject"`
	Repository repoRef `json:"repository"`
	Unread     bool    `json:"unread"`
}

func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List notifications",
		Long: `List notifications for the authenticated user. By default, only unread notifications are shown.

Use --all to include read notifications.
Use --limit to control how many notifications are returned (default 30).`,
		Aliases: []string{"ls"},
		Example: `  $ copia-cli notification list
  $ copia-cli notification list --all --limit 100`,
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

	cmd.Flags().BoolVar(&opts.All, "all", false, "Show read and unread notifications")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of notifications")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, []string{"id", "subject", "repository", "unread"})
	return cmd
}

func decodeNotifications(body []byte) ([]notification, error) {
	var notifications []notification
	err := json.Unmarshal(body, &notifications)
	return notifications, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	u := fmt.Sprintf("https://%s/api/v1/notifications", opts.Host)
	if opts.All {
		u += "?all=true"
	}
	notifications, err := cmdutil.FetchPages(opts.HTTPClient, u, opts.Token, opts.Limit, decodeNotifications)
	if err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, notifications)
	}

	if len(notifications) == 0 {
		_, _ = fmt.Fprintln(opts.IO.Out, "No unread notifications")
		return nil
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	for _, n := range notifications {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", n.Repository.FullName, n.Subject.Type, n.Subject.Title)
	}
	return w.Flush()
}
