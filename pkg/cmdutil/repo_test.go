package cmdutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo runs the test in a new git repository with the given remotes
// (name, URL pairs).
func gitRepo(t *testing.T, remotes ...string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	git(t, "init", "-q")
	for i := 0; i+1 < len(remotes); i += 2 {
		git(t, "remote", "add", remotes[i], remotes[i+1])
	}
}

func git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func TestListRemotes(t *testing.T) {
	gitRepo(t,
		"origin", "https://app.copia.io/my-fork/my-repo.git",
		"upstream", "git@app.copia.io:my-org/my-repo.git",
		"local", "not a url")
	git(t, "config", "remote.upstream.copia-resolved", "base")
	git(t, "config", "url.https://app.copia.io/my-fork/.insteadOf", "copia:")
	git(t, "remote", "set-url", "origin", "copia:my-repo.git")

	remotes, err := ListRemotes()
	require.NoError(t, err)
	assert.Equal(t, []Remote{
		{Name: "origin", Owner: "my-fork", Repo: "my-repo"},
		{Name: "upstream", Owner: "my-org", Repo: "my-repo", Resolved: true},
	}, remotes)
}

func TestListRemotes_PartialClone(t *testing.T) {
	gitRepo(t, "origin", "https://app.copia.io/my-org/my-repo.git")
	partialClone(t, "origin")

	remotes, err := ListRemotes()
	require.NoError(t, err)
	assert.Equal(t, []Remote{{Name: "origin", Owner: "my-org", Repo: "my-repo"}}, remotes)
}

// partialClone configures the remote name as a partial clone does
// (git clone --filter=blob:none), so git remote -v appends "[blob:none]".
func partialClone(t *testing.T, name string) {
	t.Helper()
	git(t, "config", "remote."+name+".promisor", "true")
	git(t, "config", "remote."+name+".partialclonefilter", "blob:none")
}

func TestListRemotes_NoRemotes(t *testing.T) {
	gitRepo(t)
	remotes, err := ListRemotes()
	require.NoError(t, err)
	assert.Empty(t, remotes)
}

func TestListRemotes_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	_, err := ListRemotes()
	assert.EqualError(t, err, "must be run from inside a git repository")
}

func TestSetDefaultRemote(t *testing.T) {
	gitRepo(t,
		"origin", "https://app.copia.io/my-fork/my-repo.git",
		"upstream", "https://app.copia.io/my-org/my-repo.git")

	require.NoError(t, SetDefaultRemote("origin"))
	assert.Equal(t, "base", git(t, "config", "remote.origin.copia-resolved"))

	require.NoError(t, SetDefaultRemote("upstream"))
	assert.Equal(t, "base", git(t, "config", "remote.upstream.copia-resolved"))
	assert.Equal(t, "remote.upstream.copia-resolved base",
		git(t, "config", "--get-regexp", `^remote\..*\.copia-resolved$`),
		"setting a new default removes the old mark")

	require.NoError(t, UnsetDefaultRemote())
	out, _ := exec.Command("git", "config", "--get-regexp", `^remote\..*\.copia-resolved$`).Output()
	assert.Empty(t, string(out))

	require.NoError(t, UnsetDefaultRemote(), "unset without a mark is not an error")
}

func TestDetectBaseRepo(t *testing.T) {
	tests := []struct {
		name      string
		remotes   []string
		resolved  string
		partial   string
		wantOwner string
		wantErr   string
	}{
		{
			name:      "origin without a default",
			remotes:   []string{"upstream", "https://app.copia.io/my-org/my-repo.git", "origin", "https://app.copia.io/my-fork/my-repo.git"},
			wantOwner: "my-fork",
		},
		{
			name:      "the default beats origin",
			remotes:   []string{"origin", "https://app.copia.io/my-fork/my-repo.git", "upstream", "https://app.copia.io/my-org/my-repo.git"},
			resolved:  "upstream",
			wantOwner: "my-org",
		},
		{
			name:      "origin of a partial clone",
			remotes:   []string{"origin", "https://app.copia.io/my-fork/my-repo.git"},
			partial:   "origin",
			wantOwner: "my-fork",
		},
		{
			name:    "neither a default nor origin",
			remotes: []string{"upstream", "https://app.copia.io/my-org/my-repo.git"},
			wantErr: "no default remote and no origin remote configured; run `copia-cli repo set-default` or use -R owner/repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitRepo(t, tt.remotes...)
			if tt.resolved != "" {
				git(t, "config", "remote."+tt.resolved+".copia-resolved", "base")
			}
			if tt.partial != "" {
				partialClone(t, tt.partial)
			}
			owner, repo, err := DetectBaseRepo()()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, "my-repo", repo)
		})
	}
}

func TestDetectBaseRepo_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	_, _, err := DetectBaseRepo()()
	assert.EqualError(t, err, "not in a git repository or no origin remote configured")
}

func TestResolveRepo_FlagBeatsDefaultRemote(t *testing.T) {
	gitRepo(t, "origin", "https://app.copia.io/my-fork/my-repo.git")
	git(t, "config", "remote.origin.copia-resolved", "base")
	f := &Factory{Repo: "other/thing", BaseRepo: DetectBaseRepo()}
	owner, repo, err := f.ResolveRepo()
	require.NoError(t, err)
	assert.Equal(t, "other", owner)
	assert.Equal(t, "thing", repo)
}
