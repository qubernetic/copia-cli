package create

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate_Web(t *testing.T) {
	const compare = "https://app.copia.io/my-org/my-repo/compare/"
	tests := []struct {
		name     string
		messages []string
		args     []string
		stdin    string
		repo     string
		tty      bool
		wantURL  string
		wantErr  string
	}{
		{name: "base and head", args: []string{"--web", "--base", "develop", "--head", "fix"}, wantURL: compare + "develop...fix?expand=1"},
		{name: "default base", args: []string{"-w", "-H", "fix"}, wantURL: compare + "main...fix?expand=1"},
		{name: "branches with slashes", args: []string{"-w", "--base", "epic/1-x", "-H", "feature/2 y#z"}, wantURL: compare + "epic/1-x...feature/2%20y%23z?expand=1"},
		{
			name: "title and body", args: []string{"-w", "-H", "fix", "-t", "fix: a & b", "-b", "line 1\nline 2"},
			wantURL: compare + "main...fix?body=line+1%0Aline+2&expand=1&title=fix%3A+a+%26+b",
		},
		{name: "body file", args: []string{"-w", "-H", "fix", "-F", "-"}, stdin: "from stdin", wantURL: compare + "main...fix?body=from+stdin&expand=1"},
		{name: "current branch", messages: []string{"feat: one"}, args: []string{"-w"}, wantURL: compare + "main...feature/x?expand=1"},
		{
			name: "fill", messages: []string{"feat: one\n\nThe body."}, args: []string{"-w", "--fill"},
			wantURL: compare + "main...feature/x?body=The+body.&expand=1&title=feat%3A+one",
		},
		{name: "repo flag", args: []string{"-w", "-H", "fix"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/compare/main...fix?expand=1"},
		{
			name: "ignored flags", args: []string{"-w", "-H", "fix", "-l", "bug", "-a", "@me", "-m", "v1"},
			wantURL: compare + "main...fix?expand=1",
			wantErr: "warning: --label is not applied in the browser\nwarning: --assignee is not applied in the browser\nwarning: --milestone is not applied in the browser\n",
		},
		{
			name: "tty", args: []string{"-w", "-H", "fix"}, tty: true, wantURL: compare + "main...fix?expand=1",
			wantErr: "Opening app.copia.io/my-org/my-repo/compare/main...fix?expand=1 in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.messages != nil {
				gitRepo(t, tt.messages...)
			}
			t.Setenv("COPIA_TOKEN", "")
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, stdin, stdout, stderr := iostreams.Test()
			stdin.WriteString(tt.stdin)
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			repo := tt.repo
			if repo == "" {
				repo = "my-org/my-repo"
			}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: repo, Browser: b}

			require.NoError(t, runCreate(t, f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdCreate_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	tests := []struct {
		name    string
		args    []string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		wantErr string
	}{
		{name: "draft", args: []string{"-w", "-H", "fix", "--draft"}, wantErr: "the `--draft` flag is not supported with `--web`"},
		{name: "reviewer", args: []string{"-w", "-H", "fix", "-r", "jane"}, wantErr: "the `--reviewer` flag is not supported with `--web`"},
		{name: "editor", args: []string{"-w", "-H", "fix", "-e"}, wantErr: "if any flags in the group [editor web] are set none of the others can be"},
		{name: "missing body file", args: []string{"-w", "-H", "fix", "-F", filepath.Join(os.TempDir(), "copia-missing.md")}, wantErr: "no such file or directory"},
		{
			name: "no config", args: []string{"-w", "-H", "fix"}, wantErr: "no config",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: &browsertest.Browser{},
					Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
			},
		},
		{
			name: "no repo", args: []string{"-w", "-H", "fix"}, wantErr: "could not determine repository",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: &browsertest.Browser{}}
			},
		},
		{
			name: "browser fails", args: []string{"-w", "-H", "fix"}, wantErr: "no browser",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
					Browser: &browsertest.Browser{Err: errors.New("no browser")}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: &browsertest.Browser{}}
			if tt.factory != nil {
				f = tt.factory(ios)
			}
			err := runCreate(t, f, tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdCreate_WebNoBranch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)

	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: b}
	err := runCreate(t, f, "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine the current branch")
	assert.Empty(t, b.URLs)

	gitRepo(t)
	require.Error(t, runCreate(t, f, "-w", "--fill", "--base", "missing"))
	assert.Empty(t, b.URLs)
}
