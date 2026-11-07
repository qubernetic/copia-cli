package clone

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// CloneOptions holds all inputs for the label clone command.
type CloneOptions struct {
	IO          *iostreams.IOStreams
	HTTPClient  *http.Client
	Host        string
	Token       string
	Owner       string
	Repo        string
	SourceOwner string
	SourceRepo  string
	Force       bool
}

type labelEntry struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type createRequest struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// updateRequest carries the description even when it is empty, so --force
// leaves the destination label as the source has it.
type updateRequest struct {
	Color       string `json:"color"`
	Description string `json:"description"`
}

// NewCmdClone creates the `copia label clone` command.
func NewCmdClone(f *cmdutil.Factory) *cobra.Command {
	opts := &CloneOptions{}

	cmd := &cobra.Command{
		Use:   "clone <source-repository>",
		Short: "Clones labels from one repository to another",
		Long: `Clones labels from a source repository to a destination repository on Copia.
By default, the destination repository is the current repository.

All labels from the source repository will be copied to the destination
repository. Labels in the destination repository that are not in the source
repository will not be deleted or modified.

Labels from the source repository that already exist in the destination
repository will be skipped. You can overwrite existing labels in the
destination repository using the --force flag.`,
		Example: `  # Clone and overwrite labels from my-org/plc-template into the current repository
  $ copia-cli label clone my-org/plc-template --force

  # Clone labels from my-org/plc-template into my-org/line-3
  $ copia-cli label clone my-org/plc-template --repo my-org/line-3`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("cannot clone labels: source-repository argument required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			sourceOwner, sourceRepo, err := cmdutil.SplitOwnerRepo(args[0])
			if err != nil {
				return err
			}
			opts.SourceOwner = sourceOwner
			opts.SourceRepo = sourceRepo

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
			return CloneRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Overwrite labels in the destination repository")

	return cmd
}

func decodeLabels(body []byte) ([]labelEntry, error) {
	var labels []labelEntry
	err := json.Unmarshal(body, &labels)
	return labels, err
}

// labelsURL is the label list of owner/repo; both are escaped, so a source
// argument cannot reach another path of the API.
func labelsURL(host, owner, repo string) string {
	return fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels", host, url.PathEscape(owner), url.PathEscape(repo))
}

// CloneRun reads every label of the source and the destination repository,
// creates the labels the destination lacks and, with Force, sets the color
// and description of the ones it has by name. Gitea does not refuse a second
// label of the same name, so existing labels are found in the list rather
// than from a create error as gh does. The first failed request aborts.
func CloneRun(opts *CloneOptions) error {
	source := opts.SourceOwner + "/" + opts.SourceRepo
	dest := opts.Owner + "/" + opts.Repo

	labels, err := cmdutil.FetchPages(opts.HTTPClient, labelsURL(opts.Host, opts.SourceOwner, opts.SourceRepo), opts.Token, 0, decodeLabels)
	if err != nil {
		return fmt.Errorf("failed to list labels of %s: %w", source, err)
	}
	destURL := labelsURL(opts.Host, opts.Owner, opts.Repo)
	existing, err := cmdutil.FetchPages(opts.HTTPClient, destURL, opts.Token, 0, decodeLabels)
	if err != nil {
		return fmt.Errorf("failed to list labels of %s: %w", dest, err)
	}
	ids := make(map[string]int64, len(existing))
	for _, l := range existing {
		if _, ok := ids[l.Name]; !ok {
			ids[l.Name] = l.ID
		}
	}

	cloned := 0
	for _, l := range labels {
		id, exists := ids[l.Name]
		switch {
		case !exists:
			err = send(opts, http.MethodPost, destURL, http.StatusCreated, createRequest{Name: l.Name, Color: l.Color, Description: l.Description})
			if err != nil {
				return fmt.Errorf("failed to create label %q: %w", l.Name, err)
			}
		case opts.Force:
			err = send(opts, http.MethodPatch, fmt.Sprintf("%s/%d", destURL, id), http.StatusOK, updateRequest{Color: l.Color, Description: l.Description})
			if err != nil {
				return fmt.Errorf("failed to update label %q: %w", l.Name, err)
			}
		default:
			continue
		}
		cloned++
	}

	if cloned == len(labels) {
		_, _ = fmt.Fprintf(opts.IO.Out, "Cloned %s from %s to %s\n", pluralize(cloned), source, dest)
	} else {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Cloned %s of %d from %s to %s\n", pluralize(cloned), len(labels), source, dest)
	}
	return nil
}

// send sends payload as JSON with method to rawURL and fails unless the
// answer is want.
func send(opts *CloneOptions, method, rawURL string, want int, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != want {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func pluralize(n int) string {
	if n == 1 {
		return "1 label"
	}
	return fmt.Sprintf("%d labels", n)
}
