package archive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// Options holds all inputs for the repo archive and repo unarchive
// commands.
type Options struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Confirmed  bool
	// Archived is the state to set: true archives, false unarchives.
	Archived bool
}

// NewCmdArchive creates the `copia repo archive` command.
func NewCmdArchive(f *cmdutil.Factory) *cobra.Command {
	return NewCmd(f, true, &cobra.Command{
		Use:   "archive [<owner/repo>]",
		Short: "Archive a repository",
		Long: `Archive a Copia repository.

With no argument, archives the current repository. There is no prompt, so
--yes is required.`,
		Example: `  # Archive the current repository
  $ copia-cli repo archive --yes

  # Archive a specific repository
  $ copia-cli repo archive my-org/my-repo --yes`,
	})
}

// NewCmd completes cmd into a command that sets the archived state of a
// repository to archived; repo unarchive uses it with false.
func NewCmd(f *cmdutil.Factory, archived bool, cmd *cobra.Command) *cobra.Command {
	opts := &Options{Archived: archived}

	cmd.Args = cobra.MaximumNArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		opts.IO = f.IOStreams
		if !opts.Confirmed {
			return errors.New("--yes required when not running interactively")
		}

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

		host, token, err := f.ResolveAuth()
		if err != nil {
			return err
		}
		opts.Host = host
		opts.Token = token

		opts.HTTPClient = cmdutil.NewHTTPClient()
		return Run(opts)
	}

	cmd.Flags().BoolVarP(&opts.Confirmed, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

// Run sets the archived state of the repository with PATCH
// /repos/{owner}/{repo}, after a GET shows it is not in that state yet.
// The command has checked --yes before.
func Run(opts *Options) error {
	repoURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", opts.Host, opts.Owner, opts.Repo)
	fullName := opts.Owner + "/" + opts.Repo

	archived, err := isArchived(opts, repoURL)
	if err != nil {
		return err
	}
	if archived == opts.Archived {
		state := "is already archived"
		if !opts.Archived {
			state = "is not archived"
		}
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Repository %s %s\n", fullName, state)
		return nil
	}

	if err := setArchived(opts, repoURL); err != nil {
		return err
	}

	verb := "Archived"
	if !opts.Archived {
		verb = "Unarchived"
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "%s repository %s\n", verb, fullName)
	return nil
}

// isArchived reads the archived field of the repository.
func isArchived(opts *Options, repoURL string) (bool, error) {
	req, err := http.NewRequest("GET", repoURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return false, fmt.Errorf("repository %s/%s not found", opts.Owner, opts.Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get repository (HTTP %d)", resp.StatusCode)
	}

	var repo struct {
		Archived bool `json:"archived"`
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, &repo); err != nil {
		return false, fmt.Errorf("parsing response: %w", err)
	}
	return repo.Archived, nil
}

// setArchived sends the archived state with PATCH /repos/{owner}/{repo}.
func setArchived(opts *Options, repoURL string) error {
	body, err := json.Marshal(map[string]bool{"archived": opts.Archived})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", repoURL, bytes.NewReader(body))
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

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	action := "archive"
	if !opts.Archived {
		action = "unarchive"
	}
	var apiErr struct {
		Message string `json:"message"`
	}
	if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("failed to %s repository (HTTP %d): %s", action, resp.StatusCode, apiErr.Message)
	}
	return fmt.Errorf("failed to %s repository (HTTP %d)", action, resp.StatusCode)
}
