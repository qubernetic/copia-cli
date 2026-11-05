package comment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/pr/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CommentOptions holds all inputs for the pr comment command.
type CommentOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Body       string
	Web        bool

	EditorMode    bool
	EditorCommand string
}

// NewCmdComment creates the `copia pr comment` command.
func NewCmdComment(f *cmdutil.Factory) *cobra.Command {
	opts := &CommentOptions{}
	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Add a comment to a pull request",
		Long: "Add a comment to a Copia pull request by number or URL.\n\n" +
			"The body comes from --body, from a file with --body-file (\"-\" reads standard input), " +
			"or from the editor with --editor. One of them, or --web, is required. On success the URL of the new comment is printed.\n\n" +
			"--editor opens the editor set in COPIA_EDITOR, the config key editor, VISUAL or EDITOR on an empty file " +
			"and posts the saved text; empty text aborts without a comment.\n\n" +
			"--web opens the pull request page in the browser to comment there. No API call is made, so no token is needed.",
		Example: `  # Comment on a pull request
  $ copia-cli pr comment 7 --body "Looks good to me"

  # Read the comment from a file, or from standard input with -
  $ copia-cli pr comment 7 --body-file review.md
  $ echo "Rebased on develop" | copia-cli pr comment https://app.copia.io/my-org/my-repo/pulls/7 -F -

  # Write the comment in the editor
  $ copia-cli pr comment 7 --editor

  # Comment in the browser
  $ copia-cli pr comment 7 --web`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := shared.ParsePRArg(args[0])
			if err != nil {
				return err
			}
			opts.Number = ref.Number
			if opts.Web {
				return shared.OpenPR(f, ref, "")
			}
			opts.IO = f.IOStreams

			if !cmd.Flags().Changed("body") && !cmd.Flags().Changed("body-file") && !opts.EditorMode {
				return fmt.Errorf("--body, --body-file, --editor or --web required")
			}
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			var owner, repo string
			if ref.Host == "" {
				owner, repo, err = f.ResolveRepo()
				if err != nil {
					return err
				}
			}
			opts.Owner, opts.Repo, err = ref.Resolve(host, owner, repo)
			if err != nil {
				return err
			}
			if opts.EditorMode {
				opts.EditorCommand = f.Editor()
			}
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return CommentRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The comment body `text`")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolVarP(&opts.EditorMode, "editor", "e", false, "Write the comment body in the text editor")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the pull request in the browser to add a comment")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")
	cmd.MarkFlagsMutuallyExclusive("body", "editor")
	cmd.MarkFlagsMutuallyExclusive("body-file", "editor")
	cmd.MarkFlagsMutuallyExclusive("body", "web")
	cmd.MarkFlagsMutuallyExclusive("body-file", "web")
	cmd.MarkFlagsMutuallyExclusive("editor", "web")

	return cmd
}

// CommentRun adds the comment with POST /repos/{owner}/{repo}/issues/{index}/comments:
// Gitea keeps pull request comments on the issue of the same index.
func CommentRun(opts *CommentOptions) error {
	if opts.EditorMode {
		body, err := cmdutil.EditBody(opts.IO, opts.EditorCommand)
		if err != nil {
			return err
		}
		opts.Body = body
	}
	if strings.TrimSpace(opts.Body) == "" {
		return fmt.Errorf("body cannot be blank")
	}
	ref := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)

	payload, _ := json.Marshal(map[string]string{"body": opts.Body})
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/comments",
		opts.Host, opts.Owner, opts.Repo, opts.Number)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
	case http.StatusNotFound:
		return fmt.Errorf("pull request %s not found", ref)
	default:
		var apiErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return fmt.Errorf("failed to add comment to pull request %s (HTTP %d): %s", ref, resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("failed to add comment to pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	var comment struct {
		HTMLURL string `json:"html_url"`
	}
	if json.Unmarshal(data, &comment) == nil && comment.HTMLURL != "" {
		_, _ = fmt.Fprintln(opts.IO.Out, comment.HTMLURL)
		return nil
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Added comment to pull request %s\n", ref)
	return nil
}
