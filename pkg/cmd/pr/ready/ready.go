package ready

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

// ReadyOptions holds all inputs for the pr ready command.
type ReadyOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Number     int64
	Undo       bool
}

// NewCmdReady creates the `copia pr ready` command.
func NewCmdReady(f *cmdutil.Factory) *cobra.Command {
	opts := &ReadyOptions{}

	cmd := &cobra.Command{
		Use:   "ready {<number> | <url>}",
		Short: "Mark a pull request as ready for review",
		Long: "Mark a draft pull request as ready for review by number or URL.\n\n" +
			"Gitea has no draft field: a pull request is a draft while its title starts with " +
			"\"WIP:\" or \"[WIP]\", in any case. The command removes that prefix from the title. " +
			"With --undo it converts the pull request back to a draft by prefixing the title with \"WIP: \".\n\n" +
			"A pull request that is already in the wanted state is left as it is. " +
			"A closed pull request cannot be changed.",
		Example: `  # Mark a draft pull request as ready for review
  $ copia-cli pr ready 7

  # Convert it back to a draft
  $ copia-cli pr ready https://app.copia.io/my-org/my-repo/pulls/7 --undo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := shared.ParsePRArg(args[0])
			if err != nil {
				return err
			}
			opts.Number = ref.Number
			opts.IO = f.IOStreams

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
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ReadyRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Undo, "undo", false, `Convert a pull request to "draft"`)

	return cmd
}

type prState struct {
	Title string `json:"title"`
	State string `json:"state"`
}

// ReadyRun changes the work-in-progress prefix of the title with
// PATCH /repos/{owner}/{repo}/pulls/{index}.
func ReadyRun(opts *ReadyOptions) error {
	prURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d",
		opts.Host, opts.Owner, opts.Repo, opts.Number)
	ref := fmt.Sprintf("%s/%s#%d", opts.Owner, opts.Repo, opts.Number)

	pr, err := fetchPR(opts, prURL, ref)
	if err != nil {
		return err
	}
	if pr.State != "open" {
		if opts.Undo {
			return fmt.Errorf(`pull request %s (%s) is closed. Only open pull requests can be converted to "draft"`, ref, pr.Title)
		}
		return fmt.Errorf(`pull request %s (%s) is closed. Only draft pull requests can be marked as "ready for review"`, ref, pr.Title)
	}

	draft := shared.HasWIPPrefix(pr.Title)
	var title, done string
	switch {
	case opts.Undo && draft:
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Pull request %s (%s) is already \"in draft\"\n", ref, pr.Title)
		return nil
	case opts.Undo:
		title, done = "WIP: "+pr.Title, `is converted to "draft"`
	case !draft:
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Pull request %s (%s) is already \"ready for review\"\n", ref, pr.Title)
		return nil
	default:
		title, done = shared.StripWIPPrefix(pr.Title), `is marked as "ready for review"`
		if strings.TrimSpace(title) == "" {
			return fmt.Errorf("pull request %s (%s) has no title without its WIP prefix; set one with `pr edit --title`", ref, pr.Title)
		}
	}

	payload, _ := json.Marshal(map[string]string{"title": title})
	resp, err := send(opts, "PATCH", prURL, payload)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var apiErr struct {
			Message string `json:"message"`
		}
		if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return fmt.Errorf("failed to update pull request %s (HTTP %d): %s", ref, resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("failed to update pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	_, _ = fmt.Fprintf(opts.IO.Out, "Pull request %s (%s) %s\n", ref, title, done)
	return nil
}

// fetchPR reads the pull request with GET /repos/{owner}/{repo}/pulls/{index}.
func fetchPR(opts *ReadyOptions, prURL, ref string) (*prState, error) {
	resp, err := send(opts, "GET", prURL, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("pull request %s not found", ref)
	default:
		return nil, fmt.Errorf("failed to read pull request %s (HTTP %d)", ref, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pr prState
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func send(opts *ReadyOptions, method, url string, payload []byte) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return opts.HTTPClient.Do(req)
}
