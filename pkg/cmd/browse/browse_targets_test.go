package browse

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoJSON = `{"default_branch":"main"}`

// noGitRepo runs the test in a directory outside of any git repository.
func noGitRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
}

// gitRepo runs the test in subdir of a new git repository on branch.
func gitRepo(t *testing.T, branch, subdir string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git(t, "init", "-q", "-b", branch)
	if subdir != "" {
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		t.Chdir(filepath.Join(root, subdir))
	}
}

func git(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, string(out))
}

type stubResponse struct {
	path   string
	status int
	body   string
}

func runTargets(t *testing.T, repo string, stubs []stubResponse, args ...string) (*stubBrowser, *httpmock.Registry, string, error) {
	t.Helper()
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	for _, s := range stubs {
		reg.Register(httpmock.REST("GET", s.path), httpmock.StringResponse(s.status, s.body))
	}

	ios, _, stdout, _ := iostreams.Test()
	browser := &stubBrowser{}
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    noConfig,
		BaseRepo:  baseRepo,
		Browser:   browser,
		Host:      "app.copia.io",
		Token:     "test-token",
		Repo:      repo,
	}
	err := runBrowseCmd(f, args...)
	return browser, reg, stdout.String(), err
}

func TestBrowse_Targets(t *testing.T) {
	getRepo := stubResponse{"/api/v1/repos/my-org/my-repo", http.StatusOK, repoJSON}
	tests := []struct {
		name     string
		args     []string
		repo     string
		stubs    []stubResponse
		wantURL  string
		wantGets []string
	}{
		{name: "settings", args: []string{"--settings"}, wantURL: "https://app.copia.io/my-org/my-repo/settings"},
		{name: "settings short", args: []string{"-s"}, wantURL: "https://app.copia.io/my-org/my-repo/settings"},
		{name: "wiki", args: []string{"-w"}, wantURL: "https://app.copia.io/my-org/my-repo/wiki"},
		{name: "releases", args: []string{"-r"}, wantURL: "https://app.copia.io/my-org/my-repo/releases"},
		{name: "actions", args: []string{"-a"}, wantURL: "https://app.copia.io/my-org/my-repo/actions"},
		{name: "projects", args: []string{"-p"}, wantURL: "https://app.copia.io/my-org/my-repo/projects"},
		{name: "number still opens an issue", args: []string{"42"}, wantURL: "https://app.copia.io/my-org/my-repo/issues/42"},
		{
			name:    "commit SHA argument",
			args:    []string{"77507cd94ccafcf568f8560cfecde965fcfa63"},
			wantURL: "https://app.copia.io/my-org/my-repo/commit/77507cd94ccafcf568f8560cfecde965fcfa63",
		},
		{
			name:     "path on the default branch",
			args:     []string{"src/main.st"},
			stubs:    []stubResponse{getRepo},
			wantURL:  "https://app.copia.io/my-org/my-repo/src/branch/main/src/main.st",
			wantGets: []string{"/api/v1/repos/my-org/my-repo"},
		},
		{
			name:     "path with a line",
			args:     []string{"src/main.st:12"},
			stubs:    []stubResponse{getRepo},
			wantURL:  "https://app.copia.io/my-org/my-repo/src/branch/main/src/main.st?display=source#L12",
			wantGets: []string{"/api/v1/repos/my-org/my-repo"},
		},
		{
			name:     "path with a line range",
			args:     []string{"src/main.st:12-20"},
			stubs:    []stubResponse{getRepo},
			wantURL:  "https://app.copia.io/my-org/my-repo/src/branch/main/src/main.st?display=source#L12-L20",
			wantGets: []string{"/api/v1/repos/my-org/my-repo"},
		},
		{
			name:    "path on a branch",
			args:    []string{"src/main.st", "--branch", "feature/x"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/branch/feature/x/src/main.st",
		},
		{
			name:    "path with a line on a branch, short flag",
			args:    []string{"main.st:3", "-b", "dev"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/main.st?display=source#L3",
		},
		{
			name:    "branch without a path",
			args:    []string{"--branch", "dev"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev",
		},
		{
			name:    "path at a commit",
			args:    []string{"main.st", "--commit=abc1234"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/commit/abc1234/main.st",
		},
		{
			name:    "commit without a path",
			args:    []string{"-c=abc1234"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/commit/abc1234",
		},
		{
			name:     "blame",
			args:     []string{"main.st", "--blame"},
			stubs:    []stubResponse{getRepo},
			wantURL:  "https://app.copia.io/my-org/my-repo/blame/branch/main/main.st",
			wantGets: []string{"/api/v1/repos/my-org/my-repo"},
		},
		{
			name:    "blame with a line on a branch",
			args:    []string{"main.st:7", "--blame", "-b", "dev"},
			wantURL: "https://app.copia.io/my-org/my-repo/blame/branch/dev/main.st#L7",
		},
		{
			name:    "blame at a commit",
			args:    []string{"main.st", "--blame", "--commit=abc1234"},
			wantURL: "https://app.copia.io/my-org/my-repo/blame/commit/abc1234/main.st",
		},
		{
			name:    "path with special characters is escaped",
			args:    []string{"PLC programs/main #1.st", "-b", "dev"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/PLC%20programs/main%20%231.st",
		},
		{
			name:    "absolute path and dot segments are cleaned",
			args:    []string{"/src/../lib/./util.st", "-b", "dev"},
			wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/lib/util.st",
		},
		{
			name:     "a number with a colon is a path",
			args:     []string{"42:1"},
			stubs:    []stubResponse{getRepo},
			wantURL:  "https://app.copia.io/my-org/my-repo/src/branch/main/42?display=source#L1",
			wantGets: []string{"/api/v1/repos/my-org/my-repo"},
		},
		{
			name: "last commit of the default branch outside a git repository",
			args: []string{"--commit"},
			stubs: []stubResponse{
				getRepo,
				{"/api/v1/repos/my-org/my-repo/branches/main", http.StatusOK, `{"name":"main","commit":{"id":"0123456789abcdef"}}`},
			},
			wantURL:  "https://app.copia.io/my-org/my-repo/src/commit/0123456789abcdef",
			wantGets: []string{"/api/v1/repos/my-org/my-repo", "/api/v1/repos/my-org/my-repo/branches/main"},
		},
		{
			name: "last commit with a path",
			args: []string{"main.st:5", "-c"},
			repo: "other-org/other-repo",
			stubs: []stubResponse{
				{"/api/v1/repos/other-org/other-repo", http.StatusOK, `{"default_branch":"trunk"}`},
				{"/api/v1/repos/other-org/other-repo/branches/trunk", http.StatusOK, `{"commit":{"id":"fedcba9876543210"}}`},
			},
			wantURL:  "https://app.copia.io/other-org/other-repo/src/commit/fedcba9876543210/main.st?display=source#L5",
			wantGets: []string{"/api/v1/repos/other-org/other-repo", "/api/v1/repos/other-org/other-repo/branches/trunk"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noGitRepo(t)
			browser, reg, _, err := runTargets(t, tt.repo, tt.stubs, tt.args...)
			require.NoError(t, err)
			assert.Equal(t, []string{tt.wantURL}, browser.urls)

			var gets []string
			for _, r := range reg.Requests {
				assert.Equal(t, "token test-token", r.Header.Get("Authorization"))
				gets = append(gets, r.URL.Path)
			}
			assert.Equal(t, tt.wantGets, gets)
		})
	}
}

func TestBrowse_LastCommitOfCurrentBranch(t *testing.T) {
	gitRepo(t, "feature/x", "")
	browser, reg, _, err := runTargets(t, "", []stubResponse{
		{"/api/v1/repos/my-org/my-repo/branches/feature/x", http.StatusOK, `{"commit":{"id":"aaaabbbbcccc"}}`},
	}, "--commit")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://app.copia.io/my-org/my-repo/src/commit/aaaabbbbcccc"}, browser.urls)
	require.Len(t, reg.Requests, 1, "one GET of the branch")
}

func TestBrowse_PathFromCurrentDirectory(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		repo    string
		wantURL string
	}{
		{name: "relative path", args: []string{"main.st", "-b", "dev"}, wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/plc/programs/main.st"},
		{name: "parent directory", args: []string{"../lib", "-b", "dev"}, wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/plc/lib"},
		{name: "current directory", args: []string{".", "-b", "dev"}, wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/plc/programs"},
		{name: "above the root is the root", args: []string{"../../..", "-b", "dev"}, wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev"},
		{name: "leading slash is the root", args: []string{"/main.st", "-b", "dev"}, wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/main.st"},
		{name: "repo flag takes the path from the root", args: []string{"main.st", "-b", "dev"}, repo: "my-org/my-repo", wantURL: "https://app.copia.io/my-org/my-repo/src/branch/dev/main.st"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitRepo(t, "main", filepath.Join("plc", "programs"))
			browser, _, _, err := runTargets(t, tt.repo, nil, tt.args...)
			require.NoError(t, err)
			assert.Equal(t, []string{tt.wantURL}, browser.urls)
		})
	}
}

func TestBrowse_NoBrowserPrintsTarget(t *testing.T) {
	noGitRepo(t)
	browser, _, out, err := runTargets(t, "", nil, "--wiki", "--no-browser")
	require.NoError(t, err)
	assert.Empty(t, browser.urls)
	assert.Equal(t, "https://app.copia.io/my-org/my-repo/wiki\n", out)
}

func TestBrowse_TargetErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		stubs   []stubResponse
		wantErr string
	}{
		{name: "argument with settings", args: []string{"main.st", "--settings"}, wantErr: "arguments not supported when using --projects, --releases, --settings, --actions or --wiki"},
		{name: "number with wiki", args: []string{"42", "-w"}, wantErr: "arguments not supported when using --projects, --releases, --settings, --actions or --wiki"},
		{name: "branch and commit", args: []string{"-b", "dev", "--commit=abc1234"}, wantErr: "specify only one of --branch, --commit, --projects, --releases, --settings, --actions or --wiki"},
		{name: "two pages", args: []string{"--releases", "--actions"}, wantErr: "specify only one of --branch, --commit, --projects, --releases, --settings, --actions or --wiki"},
		{name: "branch and projects", args: []string{"-b", "dev", "-p"}, wantErr: "specify only one of --branch, --commit, --projects, --releases, --settings, --actions or --wiki"},
		{name: "blame without a path", args: []string{"--blame"}, wantErr: "--blame requires a file path argument"},
		{name: "number with a branch", args: []string{"42", "-b", "dev"}, wantErr: `"42" is an invalid argument when using --branch or --commit`},
		{name: "commit SHA with a commit", args: []string{"abc1234", "-c"}, wantErr: `"abc1234" is an invalid argument when using --branch or --commit`},
		{name: "two colons", args: []string{"a:1:2", "-b", "dev"}, wantErr: `invalid file argument: "a:1:2"`},
		{name: "line not a number", args: []string{"a:x", "-b", "dev"}, wantErr: `invalid file argument: "a:x"`},
		{name: "line zero", args: []string{"a:0", "-b", "dev"}, wantErr: `invalid file argument: "a:0"`},
		{name: "range end not a number", args: []string{"a:1-x", "-b", "dev"}, wantErr: `invalid file argument: "a:1-x"`},
		{name: "range backwards", args: []string{"a:5-2", "-b", "dev"}, wantErr: `invalid file argument: "a:5-2"`},
		{
			name:    "repository not found",
			args:    []string{"main.st"},
			stubs:   []stubResponse{{"/api/v1/repos/my-org/my-repo", http.StatusNotFound, `{}`}},
			wantErr: "error determining the default branch: repository my-org/my-repo not found",
		},
		{
			name:    "repository without a default branch",
			args:    []string{"main.st"},
			stubs:   []stubResponse{{"/api/v1/repos/my-org/my-repo", http.StatusOK, `{}`}},
			wantErr: "error determining the default branch: repository my-org/my-repo has none",
		},
		{
			name:    "server error",
			args:    []string{"main.st"},
			stubs:   []stubResponse{{"/api/v1/repos/my-org/my-repo", http.StatusInternalServerError, `{}`}},
			wantErr: "error determining the default branch: API error (HTTP 500)",
		},
		{
			name:    "bad JSON",
			args:    []string{"main.st"},
			stubs:   []stubResponse{{"/api/v1/repos/my-org/my-repo", http.StatusOK, `{`}},
			wantErr: "error determining the default branch: parsing response: unexpected EOF",
		},
		{
			name:    "last commit: repository not found",
			args:    []string{"-c"},
			stubs:   []stubResponse{{"/api/v1/repos/my-org/my-repo", http.StatusNotFound, `{}`}},
			wantErr: "error determining the default branch: repository my-org/my-repo not found",
		},
		{
			name: "last commit: branch not found",
			args: []string{"-c"},
			stubs: []stubResponse{
				{"/api/v1/repos/my-org/my-repo", http.StatusOK, repoJSON},
				{"/api/v1/repos/my-org/my-repo/branches/main", http.StatusNotFound, `{}`},
			},
			wantErr: "branch main not found in my-org/my-repo; push it first or pass --commit=<sha>",
		},
		{
			name: "last commit: no commit in the answer",
			args: []string{"-c"},
			stubs: []stubResponse{
				{"/api/v1/repos/my-org/my-repo", http.StatusOK, repoJSON},
				{"/api/v1/repos/my-org/my-repo/branches/main", http.StatusOK, `{}`},
			},
			wantErr: "could not determine the last commit of branch main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noGitRepo(t)
			browser, reg, _, err := runTargets(t, "", tt.stubs, tt.args...)
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
			assert.Empty(t, browser.urls, "nothing opens")
			if tt.stubs == nil {
				assert.Empty(t, reg.Requests, "fails before any request")
			}
		})
	}
}

func TestBrowse_ConnectionError(t *testing.T) {
	noGitRepo(t)
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	opts := &BrowseOptions{
		IO:          ios,
		Browser:     &stubBrowser{},
		HTTPClient:  &http.Client{Transport: failingTransport{}},
		Host:        "app.copia.io",
		Owner:       "my-org",
		Repo:        "my-repo",
		SelectorArg: "main.st",
	}
	err := BrowseRun(opts)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "error determining the default branch: connecting to app.copia.io: "), err.Error())
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, assert.AnError
}
