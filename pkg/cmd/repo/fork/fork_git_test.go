package fork

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	parentURL = "https://app.copia.io/upstream-org/project.git"
	forkURL   = "https://app.copia.io/john/project.git"
	sshFork   = `{"name":"project","full_name":"john/project","html_url":"https://app.copia.io/john/project","ssh_url":"git@app.copia.io:john/project.git","owner":{"login":"john"}}`
)

// runGit runs git with the current (test) environment and fails on error.
func runGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// isolateGit keeps the user's and the system's git config out of the test.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

// workRepo creates a git repository with the given remotes (name, URL
// pairs) and chdirs into it.
func workRepo(t *testing.T, remotes ...string) {
	t.Helper()
	isolateGit(t)
	dir := t.TempDir()
	runGit(t, "init", "-q", dir)
	for i := 0; i < len(remotes); i += 2 {
		runGit(t, "-C", dir, "remote", "add", remotes[i], remotes[i+1])
	}
	t.Chdir(dir)
}

// remoteURLs returns the configured remotes as "name URL" lines in config
// order.
func remoteURLs(t *testing.T, dir string) []string {
	t.Helper()
	out := runGit(t, "-C", dir, "config", "--get-regexp", `^remote\..*\.url$`)
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(line, " ")
		lines = append(lines, strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")+" "+value)
	}
	return lines
}

// resolved returns the remote marked as the default repository, or "".
func resolved(t *testing.T, dir string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", dir, "config", "--get-regexp", `^remote\..*\.copia-resolved$`).Output()
	key, _, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	return strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".copia-resolved")
}

// forkOpts returns the options of a fork of upstream-org/project through
// reg, which answers the fork request with response unless it is empty, and
// the buffer of standard output.
func forkOpts(t *testing.T, reg *httpmock.Registry, response string) (*ForkOptions, *bytes.Buffer) {
	t.Helper()
	if response != "" {
		reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, response))
	}
	ios, _, stdout, _ := iostreams.Test()
	return &ForkOptions{
		IO: ios, HTTPClient: &http.Client{Transport: reg},
		Host: "app.copia.io", Token: "test-token",
		Owner: "upstream-org", Repo: "project",
		RemoteName: "origin", Rename: true,
	}, stdout
}

func TestForkRun_Remote(t *testing.T) {
	tests := []struct {
		name         string
		remotes      []string
		marked       string // remote marked as default before the fork
		response     string
		remoteName   string // --remote-name; empty keeps origin with Rename
		wantRemotes  []string
		wantResolved string
	}{
		{
			name:         "origin renamed to upstream",
			remotes:      []string{"origin", parentURL},
			wantRemotes:  []string{"upstream " + parentURL, "origin " + forkURL},
			wantResolved: "upstream",
		},
		{
			name:         "ssh remote gets the fork's ssh URL",
			remotes:      []string{"origin", "git@app.copia.io:upstream-org/project.git"},
			response:     sshFork,
			wantRemotes:  []string{"upstream git@app.copia.io:upstream-org/project.git", "origin git@app.copia.io:john/project.git"},
			wantResolved: "upstream",
		},
		{
			name:         "ssh remote without the fork's ssh URL gets https",
			remotes:      []string{"origin", "ssh://git@app.copia.io/upstream-org/project.git"},
			wantRemotes:  []string{"upstream ssh://git@app.copia.io/upstream-org/project.git", "origin " + forkURL},
			wantResolved: "upstream",
		},
		{
			name:         "no origin",
			remotes:      []string{"copia", parentURL},
			wantRemotes:  []string{"copia " + parentURL, "origin " + forkURL},
			wantResolved: "copia",
		},
		{
			name:         "named remote",
			remotes:      []string{"origin", parentURL},
			remoteName:   "mine",
			wantRemotes:  []string{"origin " + parentURL, "mine " + forkURL},
			wantResolved: "origin",
		},
		{
			name:         "the default mark moves with the renamed remote",
			remotes:      []string{"origin", parentURL, "other", "https://app.copia.io/other/project.git"},
			marked:       "origin",
			wantRemotes:  []string{"upstream " + parentURL, "other https://app.copia.io/other/project.git", "origin " + forkURL},
			wantResolved: "upstream",
		},
		{
			name:         "an existing default is kept",
			remotes:      []string{"origin", parentURL, "other", "https://app.copia.io/other/project.git"},
			marked:       "other",
			wantRemotes:  []string{"upstream " + parentURL, "other https://app.copia.io/other/project.git", "origin " + forkURL},
			wantResolved: "other",
		},
		{
			name:         "a remote pointing at the fork is reused",
			remotes:      []string{"origin", parentURL, "mine", "https://app.copia.io/John/project"},
			wantRemotes:  []string{"origin " + parentURL, "mine https://app.copia.io/John/project"},
			wantResolved: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workRepo(t, tt.remotes...)
			if tt.marked != "" {
				runGit(t, "config", "remote."+tt.marked+".copia-resolved", "base")
			}
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			response := tt.response
			if response == "" {
				response = johnFork
			}
			opts, _ := forkOpts(t, reg, response)
			opts.InParent, opts.Remote = true, true
			if tt.remoteName != "" {
				opts.RemoteName, opts.Rename = tt.remoteName, false
			}

			require.NoError(t, ForkRun(opts))
			assert.Equal(t, tt.wantRemotes, remoteURLs(t, "."))
			assert.Equal(t, tt.wantResolved, resolved(t, "."))
		})
	}
}

func TestForkRun_RemoteErrors(t *testing.T) {
	tests := []struct {
		name       string
		remotes    []string
		gitRepo    bool
		remoteName string
		wantErr    string
	}{
		{
			name:       "explicit remote name exists",
			gitRepo:    true,
			remotes:    []string{"origin", parentURL, "mine", "https://app.copia.io/other/project.git"},
			remoteName: "mine",
			wantErr:    "a git remote named 'mine' already exists",
		},
		{
			name:       "explicit origin exists",
			gitRepo:    true,
			remotes:    []string{"origin", parentURL},
			remoteName: "origin",
			wantErr:    "a git remote named 'origin' already exists",
		},
		{
			name:    "upstream exists",
			gitRepo: true,
			remotes: []string{"origin", parentURL, "upstream", "https://app.copia.io/other/project.git"},
			wantErr: "could not rename remote origin to upstream: ",
		},
		{
			name:       "invalid remote name",
			gitRepo:    true,
			remotes:    []string{"origin", parentURL},
			remoteName: "bad name",
			wantErr:    "failed to add remote: ",
		},
		{
			name:    "not a git repository",
			wantErr: "must be run from inside a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.gitRepo {
				workRepo(t, tt.remotes...)
			} else {
				isolateGit(t)
				t.Chdir(t.TempDir())
			}
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			opts, _ := forkOpts(t, reg, johnFork)
			opts.InParent, opts.Remote = true, true
			if tt.remoteName != "" {
				opts.RemoteName, opts.Rename = tt.remoteName, false
			}

			err := ForkRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			if tt.gitRepo {
				var want []string
				for i := 0; i < len(tt.remotes); i += 2 {
					want = append(want, tt.remotes[i]+" "+tt.remotes[i+1])
				}
				assert.Equal(t, want, remoteURLs(t, "."), "the remotes changed")
			}
		})
	}
}

func TestForkRun_FlagsForTheOtherMode(t *testing.T) {
	tests := []struct {
		name     string
		inParent bool
	}{
		{name: "clone of the current repository", inParent: true},
		{name: "remote of a repository argument", inParent: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workRepo(t, "origin", parentURL)
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			opts, _ := forkOpts(t, reg, johnFork)
			opts.InParent = tt.inParent
			if tt.inParent {
				opts.Clone = true
			} else {
				opts.Remote = true
			}

			require.NoError(t, ForkRun(opts))
			assert.Equal(t, []string{"origin " + parentURL}, remoteURLs(t, "."))
			entries, err := os.ReadDir(".")
			require.NoError(t, err)
			assert.Len(t, entries, 1, "only .git is expected")
		})
	}
}

// setupForkRemotes creates bare repositories for the parent and the fork
// under a temporary root, rewrites the Copia URLs to them with insteadOf
// rules in git's environment config, and chdirs into an empty directory.
// Missing repositories are left out.
func setupForkRemotes(t *testing.T, repos ...string) string {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	src := t.TempDir()
	runGit(t, "init", "-q", src)
	runGit(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "init")
	for _, repo := range repos {
		runGit(t, "clone", "-q", "--bare", src, filepath.Join(root, repo+".git"))
	}

	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+filepath.ToSlash(root)+"/.insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://app.copia.io/")

	work := t.TempDir()
	t.Chdir(work)
	return work
}

func TestForkRun_Clone(t *testing.T) {
	work := setupForkRemotes(t, "john/project", "upstream-org/project")
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	opts, stdout := forkOpts(t, reg, johnFork)
	opts.Clone = true

	require.NoError(t, ForkRun(opts))
	assert.Equal(t, "https://app.copia.io/john/project\n", stdout.String())

	dir := filepath.Join(work, "project")
	assert.Equal(t, []string{"origin " + forkURL, "upstream " + parentURL}, remoteURLs(t, dir))
	assert.Equal(t, "upstream", resolved(t, dir))
	assert.Contains(t, runGit(t, "-C", dir, "branch", "-r"), "upstream/")
	assert.NotContains(t, runGit(t, "-C", dir, "config", "--list"), "test-token")
}

func TestForkRun_CloneRenamedFork(t *testing.T) {
	work := setupForkRemotes(t, "my-org/renamed", "upstream-org/project")
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
	reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+orgFork+"]"))
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/project"), httpmock.StringResponse(http.StatusOK, `{}`))
	opts, _ := forkOpts(t, reg, "")
	opts.Org, opts.ForkName, opts.Clone = "my-org", "renamed", true

	require.NoError(t, ForkRun(opts))
	dir := filepath.Join(work, "renamed")
	assert.Equal(t, []string{"origin https://app.copia.io/my-org/renamed.git", "upstream " + parentURL}, remoteURLs(t, dir))
}

func TestForkRun_CloneErrors(t *testing.T) {
	tests := []struct {
		name    string
		repos   []string
		wantErr string
	}{
		{name: "fork missing", repos: []string{"upstream-org/project"}, wantErr: "failed to clone fork: "},
		{name: "parent missing", repos: []string{"john/project"}, wantErr: "exit status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupForkRemotes(t, tt.repos...)
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			opts, _ := forkOpts(t, reg, johnFork)
			opts.Clone = true

			err := ForkRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
