package view

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"full_name", "description", "html_url", "private", "default_branch", "stars_count", "forks_count", "open_issues_count"}

// ViewOptions holds all inputs for the repo view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	JSON       cmdutil.JSONFlags
}

type repoDetail struct {
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	HTMLURL         string `json:"html_url"`
	Private         bool   `json:"private"`
	DefaultBranch   string `json:"default_branch"`
	StarsCount      int    `json:"stars_count"`
	ForksCount      int    `json:"forks_count"`
	OpenIssuesCount int    `json:"open_issues_count"`
}

// NewCmdView creates the `copia repo view` command.
func NewCmdView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{}

	cmd := &cobra.Command{
		Use:   "view [<owner/repo>]",
		Short: "View a repository",
		Long:  "Display the description and other information about a repository. With no argument, the repository for the current directory is displayed.",
		Example: `  # View the current repository
  $ copia-cli repo view

  # View a specific repository
  $ copia-cli repo view my-org/my-repo

  # View as JSON
  $ copia-cli repo view --json fullName,description`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			if len(args) > 0 {
				owner, repo, err := cmdutil.SplitOwnerRepo(args[0])
				if err != nil {
					return err
				}
				opts.Owner, opts.Repo = owner, repo
			} else {
				owner, repo, err := f.ResolveRepo()
				if err != nil {
					return fmt.Errorf("could not determine repository: %w", err)
				}
				opts.Owner, opts.Repo = owner, repo
			}

			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ViewRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func ViewRun(opts *ViewOptions) error {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", opts.Host, opts.Owner, opts.Repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("repository %s/%s not found", opts.Owner, opts.Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var repo repoDetail
	if err := json.Unmarshal(body, &repo); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, repo)
	}

	visibility := "public"
	if repo.Private {
		visibility = "private"
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "%s (%s)\n", repo.FullName, visibility)
	if repo.Description != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "%s\n", repo.Description)
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "\nDefault branch: %s\n", repo.DefaultBranch)
	_, _ = fmt.Fprintf(opts.IO.Out, "Stars: %d  Forks: %d  Open issues: %d\n", repo.StarsCount, repo.ForksCount, repo.OpenIssuesCount)
	_, _ = fmt.Fprintf(opts.IO.Out, "URL: %s\n", repo.HTMLURL)

	return nil
}

