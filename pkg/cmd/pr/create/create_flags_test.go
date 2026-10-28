package create

import (
	"encoding/json"
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

// runCreateSent runs pr create with args against a mocked create endpoint
// and returns the decoded request body.
func runCreateSent(t *testing.T, ios *iostreams.IOStreams, args ...string) map[string]string {
	t.Helper()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls"),
		httpmock.StringResponse(http.StatusCreated,
			`{"number":8,"title":"x","html_url":"https://app.copia.io/my-org/my-repo/pulls/8"}`))

	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runCreate(t, f, args...))

	require.Len(t, reg.Requests, 1)
	var sent map[string]string
	require.NoError(t, json.NewDecoder(reg.Requests[0].Body).Decode(&sent))
	return sent
}

func TestCreate_Draft(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{title: "feat: x", want: "WIP: feat: x"},
		{title: "WIP: feat: x", want: "WIP: feat: x"},
		{title: "wip:feat: x", want: "wip:feat: x"},
		{title: "[WIP] feat: x", want: "[WIP] feat: x"},
		{title: "[wip] feat: x", want: "[wip] feat: x"},
		{title: "WIPE: x", want: "WIP: WIPE: x"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			sent := runCreateSent(t, ios, "--draft", "--title", tt.title, "--head", "feature/x")
			assert.Equal(t, tt.want, sent["title"])
		})
	}

	t.Run("short flag", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		sent := runCreateSent(t, ios, "-d", "-t", "feat: x", "--head", "feature/x")
		assert.Equal(t, "WIP: feat: x", sent["title"])
	})
}

// gitRepo creates a repository with one commit on main, checks out
// feature/x with one commit per message, and changes into it.
func gitRepo(t *testing.T, messages ...string) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git(t, "init", "-q", "-b", "main")
	git(t, "commit", "-q", "--allow-empty", "-m", "initial")
	git(t, "checkout", "-q", "-b", "feature/x")
	for _, m := range messages {
		git(t, "commit", "-q", "--allow-empty", "-m", m)
	}
}

func git(t *testing.T, args ...string) string {
	t.Helper()
	args = append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func TestCreate_Fill(t *testing.T) {
	tests := []struct {
		name      string
		messages  []string
		args      []string
		wantTitle string
		wantBody  string
	}{
		{
			name:      "one commit",
			messages:  []string{"feat: one\n\nFirst line.\n\nSecond line."},
			wantTitle: "feat: one",
			wantBody:  "First line.\n\nSecond line.",
		},
		{
			name:      "one commit without body",
			messages:  []string{"feat: one"},
			wantTitle: "feat: one",
		},
		{
			name:      "several commits",
			messages:  []string{"feat: a\n\nignored body", "fix: b", "docs: c"},
			wantTitle: "feature/x",
			wantBody:  "- feat: a\n- fix: b\n- docs: c",
		},
		{
			name:      "explicit title wins",
			messages:  []string{"feat: one\n\nbody"},
			args:      []string{"--title", "X"},
			wantTitle: "X",
			wantBody:  "body",
		},
		{
			name:      "explicit body wins",
			messages:  []string{"feat: a", "fix: b"},
			args:      []string{"-b", "mine"},
			wantTitle: "feature/x",
			wantBody:  "mine",
		},
		{
			name:      "draft",
			messages:  []string{"feat: a", "fix: b"},
			args:      []string{"--draft"},
			wantTitle: "WIP: feature/x",
			wantBody:  "- feat: a\n- fix: b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitRepo(t, tt.messages...)
			ios, _, _, _ := iostreams.Test()
			args := append([]string{"--fill", "--base", "main", "--head", "feature/x"}, tt.args...)
			sent := runCreateSent(t, ios, args...)
			assert.Equal(t, tt.wantTitle, sent["title"])
			assert.Equal(t, tt.wantBody, sent["body"])
			assert.Equal(t, "feature/x", sent["head"])
		})
	}

	t.Run("head defaults to current branch", func(t *testing.T) {
		gitRepo(t, "feat: one")
		ios, _, _, _ := iostreams.Test()
		sent := runCreateSent(t, ios, "-f", "--base", "main")
		assert.Equal(t, "feat: one", sent["title"])
		assert.Equal(t, "feature/x", sent["head"])
	})

	t.Run("prefers the remote-tracking base", func(t *testing.T) {
		gitRepo(t, "feat: a", "fix: b")
		git(t, "update-ref", "refs/remotes/origin/main", "HEAD~1")
		ios, _, _, _ := iostreams.Test()
		sent := runCreateSent(t, ios, "--fill", "--base", "main")
		assert.Equal(t, "fix: b", sent["title"])
	})
}

func TestCreate_FillErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		args    []string
		wantErr string
	}{
		{
			name:    "not a git repository",
			setup: func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
				t.Chdir(dir)
			},
			wantErr: "could not determine the current branch",
		},
		{
			name:    "unknown base",
			setup:   func(t *testing.T) { gitRepo(t, "feat: a") },
			args:    []string{"--base", "nope"},
			wantErr: "could not read commits between nope and feature/x",
		},
		{
			name:    "no commits",
			setup:   func(t *testing.T) { gitRepo(t) },
			wantErr: "title required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			err := runCreate(t, f, append([]string{"--fill"}, tt.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestCreate_BodyFile(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "body.md")
		require.NoError(t, os.WriteFile(path, []byte("from file\n"), 0o600))

		ios, _, _, _ := iostreams.Test()
		sent := runCreateSent(t, ios, "-t", "feat: x", "--head", "feature/x", "--body-file", path)
		assert.Equal(t, "from file\n", sent["body"])
	})

	t.Run("stdin", func(t *testing.T) {
		ios, stdin, _, _ := iostreams.Test()
		stdin.WriteString("from stdin")
		sent := runCreateSent(t, ios, "-t", "feat: x", "--head", "feature/x", "-F", "-")
		assert.Equal(t, "from stdin", sent["body"])
	})
}

func TestCreate_BodyFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "with body",
			args:    []string{"--body", "b", "--body-file", "body.md"},
			wantErr: "[body body-file] were all set",
		},
		{
			name:    "missing file",
			args:    []string{"--body-file", filepath.Join(t.TempDir(), "missing.md")},
			wantErr: "missing.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			args := append([]string{"-t", "feat: x", "--head", "feature/x"}, tt.args...)
			err := runCreate(t, f, args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}
