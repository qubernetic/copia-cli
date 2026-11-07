package fork

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/cmd/repo/clone"
	"github.com/qubernetic/copia-cli/pkg/cmd/repo/rename"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	defaultRemoteName = "origin"
	upstreamRemote    = "upstream"
)

type ForkOptions struct {
	IO                *iostreams.IOStreams
	HTTPClient        *http.Client
	Host              string
	Token             string
	Owner             string
	Repo              string
	Org               string
	ForkName          string
	DefaultBranchOnly bool
	// InParent is set when the current repository is forked (no argument
	// and no -R): then --remote applies, otherwise --clone.
	InParent   bool
	Clone      bool
	Remote     bool
	RemoteName string
	// Rename renames an existing remote named RemoteName to upstream; it is
	// set when --remote-name is not given.
	Rename bool
}

type forkRequest struct {
	Organization string `json:"organization,omitempty"`
	Name         string `json:"name,omitempty"`
}

type forkRepo struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func NewCmdFork(f *cmdutil.Factory) *cobra.Command {
	opts := &ForkOptions{}

	cmd := &cobra.Command{
		Use:   "fork [<owner/repo>]",
		Short: "Create a fork of a repository",
		Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise, forks
the specified repository. The fork is created under the authenticated user,
or in an organization with --org.

With --remote, the fork of the current repository is added as a git remote.
By default, the new fork is set to be your origin remote and any existing
origin remote is renamed to upstream. To alter this behavior, you can set a
name for the new fork's remote with --remote-name. The remote that points
at the forked repository is set as the default repository, unless one is
already set (see copia-cli repo set-default).

With --clone, the fork of the specified repository is cloned into a
directory named after it, as repo clone does, with an upstream remote for
the forked repository that is set as the default repository.

A fork that already exists is reported on standard error and used as it is.`,
		Example: `  # Fork a repository
  $ copia-cli repo fork owner/repo

  # Fork a repository and clone it locally
  $ copia-cli repo fork owner/repo --clone

  # Fork the current repository and add the fork as the origin remote
  $ copia-cli repo fork --remote

  # Fork into an organization under another name
  $ copia-cli repo fork owner/repo --org my-org --fork-name my-repo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams

			if cmd.Flags().Changed("org") && opts.Org == "" {
				return fmt.Errorf("--org cannot be blank")
			}
			if opts.RemoteName == "" {
				return fmt.Errorf("--remote-name cannot be blank")
			}
			opts.Rename = !cmd.Flags().Changed("remote-name")
			if (len(args) > 0 || f.Repo != "") && cmd.Flags().Changed("remote") {
				return fmt.Errorf("the `--remote` flag is unsupported when a repository argument is provided")
			}

			var err error
			if len(args) > 0 {
				opts.Owner, opts.Repo, err = cmdutil.SplitOwnerRepo(args[0])
				if err != nil {
					return fmt.Errorf("argument error: %w", err)
				}
			} else {
				opts.Owner, opts.Repo, err = f.ResolveRepo()
				if err != nil {
					return fmt.Errorf("unable to determine base repository: %w", err)
				}
				opts.InParent = f.Repo == ""
			}

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return ForkRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Clone, "clone", false, "Clone the fork")
	cmd.Flags().BoolVar(&opts.Remote, "remote", false, "Add a git remote for the fork")
	cmd.Flags().StringVar(&opts.RemoteName, "remote-name", defaultRemoteName, "Specify the name for the new remote")
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "Create the fork in an organization")
	cmd.Flags().StringVar(&opts.ForkName, "fork-name", "", "Rename the forked repository")
	cmd.Flags().BoolVar(&opts.DefaultBranchOnly, "default-branch-only", false, "Only include the default branch in the fork")

	return cmd
}

// ForkRun forks the repository, or finds the fork that already exists, and
// then adds it as a remote (InParent and Remote) or clones it (Clone).
func ForkRun(opts *ForkOptions) error {
	fork, created, err := createFork(opts)
	if err != nil {
		return err
	}

	if created {
		_, _ = fmt.Fprintln(opts.IO.Out, fork.HTMLURL)
		if opts.DefaultBranchOnly {
			if err := keepDefaultBranch(opts, fork); err != nil {
				return fmt.Errorf("could not delete the other branches of the fork: %w", err)
			}
		}
	} else {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "%s already exists\n", fork.FullName)
		if opts.ForkName != "" && !strings.EqualFold(fork.Name, opts.ForkName) {
			err := rename.Rename(&rename.RenameOptions{
				HTTPClient: opts.HTTPClient, Host: opts.Host, Token: opts.Token,
				Owner: fork.Owner.Login, Repo: fork.Name, NewName: opts.ForkName,
			})
			if err != nil {
				return fmt.Errorf("could not rename fork: %w", err)
			}
			fork.Name = opts.ForkName
			fork.FullName = fork.Owner.Login + "/" + opts.ForkName
		}
	}

	switch {
	case opts.InParent && opts.Remote:
		return addRemote(opts, fork)
	case !opts.InParent && opts.Clone:
		return cloneFork(opts, fork)
	}
	return nil
}

// createFork sends POST /repos/{owner}/{repo}/forks. Gitea answers 409
// when the owner already has a fork (or a repository of that name); then
// the existing fork is looked up and returned with created false.
func createFork(opts *ForkOptions) (forkRepo, bool, error) {
	var fork forkRepo
	body, err := json.Marshal(forkRequest{Organization: opts.Org, Name: opts.ForkName})
	if err != nil {
		return fork, false, err
	}

	forksURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/forks", opts.Host, opts.Owner, opts.Repo)
	resp, err := do(opts, "POST", forksURL, bytes.NewReader(body))
	if err != nil {
		return fork, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fork, false, err
	}

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusCreated:
		err := json.Unmarshal(respBody, &fork)
		return fork, true, err
	case http.StatusConflict:
		existing, found, err := findFork(opts, forksURL)
		if err != nil {
			return fork, false, err
		}
		if found {
			return existing, false, nil
		}
	}
	return fork, false, fmt.Errorf("failed to fork repository (HTTP %d): %s", resp.StatusCode, string(respBody))
}

// findFork returns the fork of the repository owned by --org or, without
// it, by the authenticated user.
func findFork(opts *ForkOptions, forksURL string) (forkRepo, bool, error) {
	owner := opts.Org
	if owner == "" {
		login, err := currentLogin(opts)
		if err != nil {
			return forkRepo{}, false, err
		}
		owner = login
	}

	forks, err := cmdutil.FetchFilteredPages(opts.HTTPClient, forksURL, opts.Token, 1,
		func(body []byte) ([]forkRepo, error) {
			var page []forkRepo
			err := json.Unmarshal(body, &page)
			return page, err
		},
		func(r forkRepo) bool { return strings.EqualFold(r.Owner.Login, owner) })
	if err != nil {
		return forkRepo{}, false, fmt.Errorf("failed to list forks: %w", err)
	}
	if len(forks) == 0 {
		return forkRepo{}, false, nil
	}
	return forks[0], true, nil
}

func currentLogin(opts *ForkOptions) (string, error) {
	resp, err := do(opts, "GET", fmt.Sprintf("https://%s/api/v1/user", opts.Host), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get the authenticated user (HTTP %d)", resp.StatusCode)
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	return user.Login, nil
}

// keepDefaultBranch deletes every branch of a new fork but its default
// branch, since Gitea's fork API copies all of them.
func keepDefaultBranch(opts *ForkOptions, fork forkRepo) error {
	repoURL := fmt.Sprintf("https://%s/api/v1/repos/%s", opts.Host, fork.FullName)
	branches, err := cmdutil.FetchFilteredPages(opts.HTTPClient, repoURL+"/branches", opts.Token, 0,
		func(body []byte) ([]string, error) {
			var page []struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(body, &page); err != nil {
				return nil, err
			}
			names := make([]string, len(page))
			for i, b := range page {
				names[i] = b.Name
			}
			return names, nil
		},
		func(name string) bool { return name != fork.DefaultBranch })
	if err != nil {
		return err
	}

	for _, name := range branches {
		segments := strings.Split(name, "/")
		for i, s := range segments {
			segments[i] = url.PathEscape(s)
		}
		resp, err := do(opts, "DELETE", repoURL+"/branches/"+strings.Join(segments, "/"), nil)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("deleting branch %s (HTTP %d)", name, resp.StatusCode)
		}
	}
	return nil
}

func do(opts *ForkOptions, method, rawURL string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	return resp, nil
}

// addRemote adds the fork as the remote RemoteName of the current
// repository. A remote that already points at the fork is kept. An existing
// remote named RemoteName is renamed to upstream when Rename is set and is
// an error otherwise. When no remote is the default repository yet, the
// remote of the forked repository becomes it, so the commands that work on
// the current repository keep using the forked repository, not the fork.
func addRemote(opts *ForkOptions, fork forkRepo) error {
	remotes, err := cmdutil.ListRemotes()
	if err != nil {
		return err
	}
	parent := opts.Owner + "/" + opts.Repo
	parentURL := ""
	for _, r := range remotes {
		if strings.EqualFold(r.FullName(), fork.FullName) {
			return nil
		}
		if parentURL == "" && strings.EqualFold(r.FullName(), parent) {
			parentURL, _ = git("config", "--get", "remote."+r.Name+".url")
		}
	}

	names, err := git("remote")
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(names) {
		if name != opts.RemoteName {
			continue
		}
		if !opts.Rename {
			return fmt.Errorf("a git remote named '%s' already exists", opts.RemoteName)
		}
		if _, err := git("remote", "rename", opts.RemoteName, upstreamRemote); err != nil {
			return fmt.Errorf("could not rename remote %s to %s: %w", opts.RemoteName, upstreamRemote, err)
		}
	}

	if _, err := git("remote", "add", opts.RemoteName, forkRemoteURL(opts.Host, parentURL, fork)); err != nil {
		return fmt.Errorf("failed to add remote: %w", err)
	}

	remotes, err = cmdutil.ListRemotes()
	if err != nil {
		return err
	}
	for _, r := range remotes {
		if r.Resolved {
			return nil
		}
	}
	for _, r := range remotes {
		if strings.EqualFold(r.FullName(), parent) {
			return cmdutil.SetDefaultRemote(r.Name)
		}
	}
	return nil
}

// forkRemoteURL returns the fork's SSH URL when the forked repository's
// remote uses SSH, as gh follows the remote's protocol, otherwise the https
// URL on the configured host, which repo clone uses as well.
func forkRemoteURL(host, parentURL string, fork forkRepo) string {
	ssh := strings.HasPrefix(parentURL, "ssh://") ||
		(parentURL != "" && !strings.Contains(parentURL, "://") && strings.Contains(parentURL, ":"))
	if ssh && fork.SSHURL != "" {
		return fork.SSHURL
	}
	return fmt.Sprintf("https://%s/%s.git", host, fork.FullName)
}

// cloneFork clones the fork into a directory named after it, as repo clone
// does, adds the forked repository as the remote upstream, marks it as the
// default repository and fetches it.
func cloneFork(opts *ForkOptions, fork forkRepo) error {
	dir := fork.Name
	err := clone.CloneRun(&clone.CloneOptions{
		IO: opts.IO, Host: opts.Host, Token: opts.Token, Repo: fork.FullName, Dir: dir,
	})
	if err != nil {
		return fmt.Errorf("failed to clone fork: %w", err)
	}

	upstreamURL := fmt.Sprintf("https://%s/%s/%s.git", opts.Host, opts.Owner, opts.Repo)
	if _, err := git("-C", dir, "remote", "add", upstreamRemote, upstreamURL); err != nil {
		return err
	}
	if _, err := git("-C", dir, "config", "remote."+upstreamRemote+"."+cmdutil.RemoteResolvedKey, "base"); err != nil {
		return err
	}
	return clone.RunGit(opts.IO, opts.Host, opts.Token, upstreamURL, "-C", dir, "fetch", upstreamRemote)
}

// git runs a local git command and returns its trimmed output; on failure
// the error holds git's message.
func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(out) > 0 {
			return "", errors.New(strings.TrimSpace(string(out)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
