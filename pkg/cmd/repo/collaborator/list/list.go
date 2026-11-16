package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"login", "full_name"}

// ListOptions holds all inputs for the repo collaborator list command.
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

// Collaborator is one collaborator of a repository. It has no permission:
// Copia does not register Gitea's route for one collaborator's permission.
type Collaborator struct {
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// NewCmdList creates the `copia-cli repo collaborator list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List collaborators of a repository",
		Long: `List the collaborators of a Copia repository with their login and name.

Use --limit to control how many collaborators are returned (default 30).`,
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Example: `  $ copia-cli repo collaborator list
  $ copia-cli repo collaborator list -R my-org/my-repo --limit 100
  $ copia-cli repo collaborator list --json login,full_name`,
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

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of collaborators")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func decodeCollaborators(body []byte) ([]Collaborator, error) {
	var users []Collaborator
	err := json.Unmarshal(body, &users)
	return users, err
}

func ListRun(opts *ListOptions) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}

	base := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/collaborators", opts.Host, url.PathEscape(opts.Owner), url.PathEscape(opts.Repo))
	collaborators, err := cmdutil.FetchPages(opts.HTTPClient, base, opts.Token, opts.Limit, decodeCollaborators)
	if err != nil {
		var httpErr *cmdutil.HTTPError
		if errors.As(err, &httpErr) {
			return apiError("failed to list collaborators", httpErr.StatusCode, httpErr.Message)
		}
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, collaborators)
	}

	if len(collaborators) == 0 {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "no collaborators found in %s/%s\n", opts.Owner, opts.Repo)
		return nil
	}

	if err := opts.IO.StartPager(); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if !opts.IO.IsStdoutTTY() {
		for _, c := range collaborators {
			_, _ = fmt.Fprintf(opts.IO.Out, "%s\t%s\n", c.Login, c.FullName)
		}
		return nil
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "LOGIN\tNAME")
	for _, c := range collaborators {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", c.Login, c.FullName)
	}
	return w.Flush()
}

// apiError words a refusal of the API with its HTTP status and, when there
// is one, Gitea's message.
func apiError(what string, status int, message string) error {
	if message != "" {
		return fmt.Errorf("%s (HTTP %d): %s", what, status, message)
	}
	return fmt.Errorf("%s (HTTP %d)", what, status)
}
