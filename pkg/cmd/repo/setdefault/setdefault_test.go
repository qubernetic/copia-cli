package setdefault

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	forkURL     = "https://app.copia.io/my-fork/my-repo.git"
	upstreamURL = "git@app.copia.io:my-org/my-repo.git"
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

// marks returns the copia-resolved keys of the repository, one per line.
func marks(t *testing.T) string {
	t.Helper()
	out, _ := exec.Command("git", "config", "--get-regexp", `^remote\..*\.copia-resolved$`).Output()
	return strings.TrimSpace(string(out))
}

func runCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	ios, _, stdout, stderr := iostreams.Test()
	cmd := NewCmdSetDefault(&cmdutil.Factory{IOStreams: ios})
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestSetDefault_Set(t *testing.T) {
	tests := []struct {
		name     string
		arg      string
		wantMark string
		wantRepo string
	}{
		{"owner/repo", "my-org/my-repo", "upstream", "my-org/my-repo"},
		{"owner/repo in another case", "My-Fork/My-Repo", "origin", "my-fork/my-repo"},
		{"remote name", "upstream", "upstream", "my-org/my-repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitRepo(t, "origin", forkURL, "upstream", upstreamURL)
			stdout, _, err := runCmd(t, tt.arg)
			require.NoError(t, err)
			assert.Equal(t, "Set "+tt.wantRepo+" as the default repository for the current directory\n", stdout)
			assert.Equal(t, "remote."+tt.wantMark+".copia-resolved base", marks(t))
		})
	}
}

func TestSetDefault_SetReplacesTheOldDefault(t *testing.T) {
	gitRepo(t, "origin", forkURL, "upstream", upstreamURL)
	git(t, "config", "remote.origin.copia-resolved", "base")

	_, _, err := runCmd(t, "my-org/my-repo")
	require.NoError(t, err)
	assert.Equal(t, "remote.upstream.copia-resolved base", marks(t))
}

func TestSetDefault_OneRemoteWithoutArgument(t *testing.T) {
	gitRepo(t, "origin", forkURL)
	stdout, _, err := runCmd(t)
	require.NoError(t, err)
	assert.Equal(t, "Found only one known remote repo, my-fork/my-repo.\n"+
		"Set my-fork/my-repo as the default repository for the current directory\n", stdout)
	assert.Equal(t, "remote.origin.copia-resolved base", marks(t))
}

func TestSetDefault_Errors(t *testing.T) {
	tests := []struct {
		name    string
		remotes []string
		args    []string
		wantErr string
	}{
		{
			name:    "no matching remote",
			remotes: []string{"origin", forkURL},
			args:    []string{"other/repo"},
			wantErr: "other/repo does not correspond to any git remotes",
		},
		{
			name:    "unknown remote name",
			remotes: []string{"origin", forkURL},
			args:    []string{"upstream"},
			wantErr: "upstream does not correspond to any git remotes",
		},
		{
			name:    "several remotes without an argument",
			remotes: []string{"origin", forkURL, "upstream", upstreamURL},
			wantErr: "repository required when there are several git remotes:\n" +
				"  origin    my-fork/my-repo\n" +
				"  upstream  my-org/my-repo",
		},
		{
			name:    "no remotes",
			wantErr: "none of the git remotes correspond to a valid remote repository",
		},
		{
			name:    "argument with --view",
			remotes: []string{"origin", forkURL},
			args:    []string{"my-fork/my-repo", "--view"},
			wantErr: "the repository argument cannot be used with --view or --unset",
		},
		{
			name:    "--view with --unset",
			remotes: []string{"origin", forkURL},
			args:    []string{"--view", "--unset"},
			wantErr: "if any flags in the group [view unset] are set none of the others can be; [unset view] were all set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitRepo(t, tt.remotes...)
			_, _, err := runCmd(t, tt.args...)
			assert.EqualError(t, err, tt.wantErr)
			assert.Empty(t, marks(t))
		})
	}
}

func TestSetDefault_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	for _, args := range [][]string{{"my-org/my-repo"}, {"--view"}, {"--unset"}} {
		_, _, err := runCmd(t, args...)
		assert.EqualError(t, err, "must be run from inside a git repository", args)
	}
}

func TestSetDefault_View(t *testing.T) {
	gitRepo(t, "origin", forkURL, "upstream", upstreamURL)

	stdout, stderr, err := runCmd(t, "--view")
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "! No default remote repository has been set. To learn more about the default repository, run: copia-cli repo set-default --help\n", stderr)

	git(t, "config", "remote.upstream.copia-resolved", "base")
	stdout, stderr, err = runCmd(t, "-v")
	require.NoError(t, err)
	assert.Equal(t, "my-org/my-repo\n", stdout)
	assert.Empty(t, stderr)
}

func TestSetDefault_Unset(t *testing.T) {
	gitRepo(t, "origin", forkURL, "upstream", upstreamURL)
	git(t, "config", "remote.upstream.copia-resolved", "base")

	stdout, _, err := runCmd(t, "--unset")
	require.NoError(t, err)
	assert.Equal(t, "Unset my-org/my-repo as default repository\n", stdout)
	assert.Empty(t, marks(t))

	stdout, stderr, err := runCmd(t, "-u")
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "! No default repository has been set\n", stderr)
}

func TestSetDefault_UnsetRemovesAMarkOnAnUnparsableRemote(t *testing.T) {
	gitRepo(t, "origin", forkURL, "local", "file:///my-repo")
	git(t, "config", "remote.local.copia-resolved", "base")

	stdout, stderr, err := runCmd(t, "--unset")
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "! No default repository has been set\n", stderr)
	assert.Empty(t, marks(t))
}
