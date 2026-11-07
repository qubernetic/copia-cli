package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmd/release/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

var errNoChange = errors.New("use flags to specify properties to edit")

// EditOptions holds all inputs for the release edit command. A nil pointer
// and an empty string leave the field as it is.
type EditOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Tag        string
	NewTag     string
	Target     string
	Title      *string
	Notes      *string
	Draft      *bool
	Prerelease *bool
}

type editRequest struct {
	TagName         string  `json:"tag_name,omitempty"`
	TargetCommitish string  `json:"target_commitish,omitempty"`
	Name            *string `json:"name,omitempty"`
	Body            *string `json:"body,omitempty"`
	Draft           *bool   `json:"draft,omitempty"`
	Prerelease      *bool   `json:"prerelease,omitempty"`
}

// editFlags are the flags that change a release.
var editFlags = []string{"draft", "prerelease", "title", "notes", "notes-file", "tag", "target"}

// NewCmdEdit creates the `copia-cli release edit` command.
func NewCmdEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &EditOptions{}
	var title, notes, notesFile string
	var draft, prerelease bool

	cmd := &cobra.Command{
		Use:   "edit <tag>",
		Short: "Edit a release",
		Long: `Edit a release of a Copia repository.

Only the properties given as flags change. --notes-file reads the notes from
a file; "-" reads standard input. On success the release URL is printed.`,
		Example: `  # Publish a release that was previously a draft
  $ copia-cli release edit v1.0 --draft=false

  # Update the release notes from the content of a file
  $ copia-cli release edit v1.0 --notes-file /path/to/release_notes.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Tag = args[0]

			changed := false
			for _, name := range editFlags {
				changed = changed || cmd.Flags().Changed(name)
			}
			if !changed {
				return errNoChange
			}
			if cmd.Flags().Changed("draft") {
				opts.Draft = &draft
			}
			if cmd.Flags().Changed("prerelease") {
				opts.Prerelease = &prerelease
			}
			if cmd.Flags().Changed("title") {
				opts.Title = &title
			}
			if cmd.Flags().Changed("notes") {
				opts.Notes = &notes
			}
			if notesFile != "" {
				b, err := cmdutil.ReadFile(notesFile, opts.IO.In)
				if err != nil {
					return err
				}
				body := string(b)
				opts.Notes = &body
			}

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
			return EditRun(opts)
		},
	}

	cmd.Flags().BoolVar(&draft, "draft", false, "Save the release as a draft instead of publishing it")
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "Mark the release as a prerelease")
	cmd.Flags().StringVarP(&notes, "notes", "n", "", "Release notes")
	cmd.Flags().StringVarP(&title, "title", "t", "", "Release title")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target `branch` or full commit SHA")
	cmd.Flags().StringVar(&opts.NewTag, "tag", "", "The name of the tag")
	cmd.Flags().StringVarP(&notesFile, "notes-file", "F", "", "Read release notes from `file` (use \"-\" to read from standard input)")

	return cmd
}

// EditRun looks the release up by tag and sends the given fields with
// PATCH /repos/{owner}/{repo}/releases/{id}, then prints the release URL.
func EditRun(opts *EditOptions) error {
	payload := editRequest{
		TagName:         opts.NewTag,
		TargetCommitish: opts.Target,
		Name:            opts.Title,
		Body:            opts.Notes,
		Draft:           opts.Draft,
		Prerelease:      opts.Prerelease,
	}
	if payload == (editRequest{}) {
		return errNoChange
	}

	release, err := shared.FetchRelease(opts.HTTPClient, opts.Host, opts.Token, opts.Owner, opts.Repo, opts.Tag)
	if err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/%d", opts.Host, opts.Owner, opts.Repo, release.ID)
	req, err := http.NewRequest("PATCH", u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to edit release (HTTP %d)", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var edited shared.Release
	if err := json.Unmarshal(respBody, &edited); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(opts.IO.Out, edited.HTMLURL)
	return nil
}
