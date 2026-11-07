package rename

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

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// RenameOptions holds all inputs for the repo rename command.
type RenameOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	NewName    string
	Confirmed  bool
	// UpdateRemote points the git remote of the current directory that
	// names the repository at the new name; -R leaves the remotes alone.
	UpdateRemote bool
}

// NewCmdRename creates the `copia repo rename` command.
func NewCmdRename(f *cmdutil.Factory) *cobra.Command {
	opts := &RenameOptions{}

	cmd := &cobra.Command{
		Use:   "rename <new-name>",
		Short: "Rename a repository",
		Long: `Rename a Copia repository.

<new-name> is the new repository name without the owner. A repository cannot
move to another owner this way.

By default, the current repository is renamed. Otherwise, the repository
specified with -R/--repo is renamed. There is no prompt, so --yes is required.`,
		Example: `  # Rename the current repository (my-org/old -> my-org/new)
  $ copia-cli repo rename new --yes

  # Rename the specified repository (my-org/old -> my-org/new)
  $ copia-cli repo rename -R my-org/old new --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.NewName = args[0]

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return fmt.Errorf("could not determine repository: %w", err)
			}
			opts.Owner, opts.Repo = owner, repo
			opts.UpdateRemote = f.Repo == ""

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			opts.HTTPClient = cmdutil.NewHTTPClient()
			return RenameRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Confirmed, "yes", "y", false, "Confirm the rename without prompting")

	return cmd
}

// RenameRun renames the repository with PATCH /repos/{owner}/{repo}.
func RenameRun(opts *RenameOptions) error {
	if strings.Contains(opts.NewName, "/") {
		return errors.New("New repository name cannot contain '/' character - to transfer a repository to a new owner, see <https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository>.") //nolint:staticcheck // gh's message, word for word
	}
	if !opts.Confirmed {
		return fmt.Errorf("renaming %s/%s to %s needs confirmation; use --yes to confirm", opts.Owner, opts.Repo, opts.NewName)
	}

	if err := Rename(opts); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Renamed repository %s/%s\n", opts.Owner, opts.NewName)

	if !opts.UpdateRemote {
		return nil
	}
	// The repository is renamed, so a remote that cannot follow is a
	// warning, not a failure.
	remote, err := updateRemote(opts)
	switch {
	case err != nil && remote != "":
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Warning: unable to update remote %q: %v\n", remote, err)
	case err != nil:
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "! Warning: unable to update remote: %v\n", err)
	default:
		_, _ = fmt.Fprintf(opts.IO.Out, "Updated the %q remote\n", remote)
	}
	return nil
}

// updateRemote sets the URL of the git remote that points at the old
// repository on the configured host to the new name, keeping the rest of
// the URL (scheme, user, host, a .git suffix). It prefers the remote marked
// by `repo set-default`, then origin, then the first in config order, and
// returns the remote's name, empty when none was found.
func updateRemote(opts *RenameOptions) (string, error) {
	remotes, err := cmdutil.ListRemotes()
	if err != nil {
		return "", err
	}

	var name, rawURL string
	best := 3
	for _, r := range remotes {
		if !strings.EqualFold(r.Owner, opts.Owner) || !strings.EqualFold(r.Repo, opts.Repo) {
			continue
		}
		// The URL as configured, before any insteadOf rule, is what
		// set-url writes back.
		out, err := exec.Command("git", "config", "--get", "remote."+r.Name+".url").Output()
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(out))
		if !sameHostName(remoteHost(raw), opts.Host) {
			continue
		}
		rank := 2
		if r.Resolved {
			rank = 0
		} else if r.Name == "origin" {
			rank = 1
		}
		if rank < best {
			best, name, rawURL = rank, r.Name, raw
		}
	}
	if name == "" {
		return "", fmt.Errorf("no git remote points at %s/%s on %s", opts.Owner, opts.Repo, opts.Host)
	}

	newURL, ok := renameURL(rawURL, opts.Repo, opts.NewName)
	if !ok {
		return name, fmt.Errorf("the remote URL does not end in the repository name")
	}
	if out, err := exec.Command("git", "remote", "set-url", name, newURL).CombinedOutput(); err != nil {
		return name, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return name, nil
}
// renameURL replaces the last path segment of a remote URL, the repository
// name with an optional .git suffix and trailing slash, with newName.
func renameURL(raw, oldName, newName string) (string, bool) {
	rest := strings.TrimSuffix(raw, "/")
	suffix := ""
	if strings.HasSuffix(strings.ToLower(rest), ".git") {
		rest, suffix = rest[:len(rest)-4], rest[len(rest)-4:]
	}
	i := strings.LastIndexAny(rest, "/:")
	if i < 0 || !strings.EqualFold(rest[i+1:], oldName) {
		return "", false
	}
	return rest[:i+1] + newName + suffix, true
}

// remoteHost returns the host name of a git remote URL, either a URL with a
// scheme or the scp-like form [user@]host:path, without user and port.
func remoteHost(raw string) string {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return u.Hostname()
	}
	host, _, ok := strings.Cut(raw, ":")
	if !ok {
		return ""
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	return host
}

// sameHostName reports whether the remote's host name is the configured
// host's, ignoring the port (an ssh remote uses another one) and case.
// Matching only picks the remote to update; no token goes anywhere.
func sameHostName(remote, configured string) bool {
	u, err := cmdutil.ParseHost(configured)
	if err != nil || remote == "" {
		return false
	}
	return strings.EqualFold(remote, u.Hostname())
}

// Rename sends the new name with PATCH /repos/{owner}/{repo}; repo fork
// uses it to rename an existing fork.
func Rename(opts *RenameOptions) error {
	body, err := json.Marshal(map[string]string{"name": opts.NewName})
	if err != nil {
		return err
	}
	repoURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s", opts.Host, opts.Owner, opts.Repo)
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
	var apiErr struct {
		Message string `json:"message"`
	}
	if data, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("failed to rename repository (HTTP %d): %s", resp.StatusCode, apiErr.Message)
	}
	return fmt.Errorf("failed to rename repository (HTTP %d)", resp.StatusCode)
}
