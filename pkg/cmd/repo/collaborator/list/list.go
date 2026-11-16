package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"login", "full_name", "permission"}

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

// Collaborator is one collaborator of a repository with its permission.
type Collaborator struct {
	Login      string `json:"login"`
	FullName   string `json:"full_name"`
	Permission string `json:"permission"`
}

// NewCmdList creates the `copia-cli repo collaborator list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List collaborators of a repository",
		Long: `List the collaborators of a Copia repository with their login, name and
permission.

Use --limit to control how many collaborators are returned (default 30).`,
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Example: `  $ copia-cli repo collaborator list
  $ copia-cli repo collaborator list -R my-org/my-repo --limit 100
  $ copia-cli repo collaborator list --json login,permission`,
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

	// The collaborators route gives users only; the permission of each is
	// a request of its own.
	for i := range collaborators {
		permission, err := fetchPermission(opts, base, collaborators[i].Login)
		if err != nil {
			return err
		}
		collaborators[i].Permission = permission
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
			_, _ = fmt.Fprintf(opts.IO.Out, "%s\t%s\t%s\n", c.Login, c.FullName, c.Permission)
		}
		return nil
	}

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "LOGIN\tNAME\tPERMISSION")
	for _, c := range collaborators {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", c.Login, c.FullName, c.Permission)
	}
	return w.Flush()
}

// fetchPermission reads the permission of the collaborator login with
// GET /repos/{owner}/{repo}/collaborators/{login}/permission.
func fetchPermission(opts *ListOptions, base, login string) (string, error) {
	req, err := http.NewRequest("GET", base+"/"+url.PathEscape(login)+"/permission", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", apiError("failed to read the permission of "+login, resp.StatusCode, cmdutil.ReadAPIMessage(resp.Body))
	}

	var perm struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, cmdutil.MaxErrorBody)).Decode(&perm); err != nil {
		return "", fmt.Errorf("parsing the permission of %s: %w", login, err)
	}
	return perm.Permission, nil
}

// apiError words a refusal of the API with its HTTP status and, when there
// is one, Gitea's message.
func apiError(what string, status int, message string) error {
	if message != "" {
		return fmt.Errorf("%s (HTTP %d): %s", what, status, message)
	}
	return fmt.Errorf("%s (HTTP %d)", what, status)
}
