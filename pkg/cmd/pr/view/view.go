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

var validJSONFields = []string{"number", "title", "body", "state", "mergeable", "user", "base", "head", "created_at", "html_url"}

// ViewOptions holds all inputs for the pr view command.
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

type branchRef struct {
	Label string `json:"label"`
}

type prDetail struct {
	Number    int64     `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Mergeable bool      `json:"mergeable"`
	HTMLURL   string    `json:"html_url"`
	User      userRef   `json:"user"`
	Base      branchRef `json:"base"`
	Head      branchRef `json:"head"`
	CreatedAt string    `json:"created_at"`
}

// NewCmdView creates the `copia pr view` command.
func NewCmdView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{}

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View a pull request",
		Long:  "Display the title, body, and other information about a pull request.",
		Example: `  # View a pull request
  $ copia-cli pr view 7

  # View a pull request with its comments
  $ copia-cli pr view 7 --comments

  # View as JSON
  $ copia-cli pr view 7 --json number,title,mergeable`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Comments && opts.JSON.IsJSON() {
				return fmt.Errorf("specify only one of --comments or --json")
			}

			num, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid PR number: %s", args[0])
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
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d",
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
		return fmt.Errorf("PR #%d not found", opts.Number)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var pr prDetail
	if err := json.Unmarshal(body, &pr); err != nil {
		return err
	}

	if opts.JSON.IsJSON() {
		return opts.JSON.Print(opts.IO.Out, pr)
	}

	var comments []shared.Comment
	if opts.Comments {
		comments, err = shared.FetchComments(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Number)
		if err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "#%d %s\n", pr.Number, pr.Title)
	_, _ = fmt.Fprintf(opts.IO.Out, "State: %s  Author: %s  Mergeable: %v\n", pr.State, pr.User.Login, pr.Mergeable)
	_, _ = fmt.Fprintf(opts.IO.Out, "Branches: %s <- %s\n", pr.Base.Label, pr.Head.Label)

	if pr.Body != "" {
		_, _ = fmt.Fprintf(opts.IO.Out, "\n%s\n", pr.Body)
	}

	shared.PrintComments(opts.IO.Out, comments)

	_, _ = fmt.Fprintf(opts.IO.Out, "\n%s\n", pr.HTMLURL)
	return nil
}
