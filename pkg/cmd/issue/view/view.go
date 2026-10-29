package view

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/issue/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var validJSONFields = []string{"number", "title", "body", "state", "user", "labels", "created_at", "comments", "html_url"}

// ViewOptions holds all inputs for the issue view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	JSON       cmdutil.JSONFlags
	Comments   bool
}

type userRef struct {
	Login string `json:"login"`
}

type labelRef struct {
	Name string `json:"name"`
}

type issueDetail struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	User      userRef    `json:"user"`
	Labels    []labelRef `json:"labels"`
	CreatedAt string     `json:"created_at"`
	Comments  int        `json:"comments"`
}

// NewCmdView creates the `copia issue view` command.
func NewCmdView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{}

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View an issue",
		Long:  "Display the title, body, and other information about an issue.",
		Example: `  # View an issue
  $ copia-cli issue view 12

  # View an issue with its comments
  $ copia-cli issue view 12 --comments

  # View as JSON
  $ copia-cli issue view 12 --json number,title,state`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Comments && opts.JSON.IsJSON() {
				return fmt.Errorf("specify only one of --comments or --json")
			}

			num, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid issue number: %s", args[0])
			}
			opts.Number = num
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
			return ViewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "View comments")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, validJSONFields)

	return cmd
}

func ViewRun(opts *ViewOptions) error {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

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
		return fmt.Errorf("issue #%d not found", opts.Number)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var issue issueDetail
	if err := json.Unmarshal(body, &issue); err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, issue)
	}

	var comments []shared.Comment
	if opts.Comments {
		comments, err = shared.FetchComments(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Number)
		if err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "#%d %s\n", issue.Number, issue.Title)
	_, _ = fmt.Fprintf(opts.IO.Out, "State: %s  Author: %s  Comments: %d\n", issue.State, issue.User.Login, issue.Comments)

	labels := ""
	for i, l := range issue.Labels {
		if i > 0 {
			labels += ", "
		}
		labels += l.Name
	}
	if labels != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "Labels: %s\n", labels)
	}

	if issue.Body != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "\n%s\n", issue.Body)
	}

	shared.PrintComments(opts.IO.Out, comments)

	_, _ = fmt.Fprintf(opts.IO.Out, "\n%s\n", issue.HTMLURL)
	return nil
}
