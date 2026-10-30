package browse

import (
	"errors"
	"io"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubBrowser struct {
	urls []string
	err  error
}

func (b *stubBrowser) Browse(url string) error {
	b.urls = append(b.urls, url)
	return b.err
}

func baseRepo() (string, string, error) { return "my-org", "my-repo", nil }

func noConfig() (*config.Config, error) {
	return &config.Config{Hosts: map[string]*config.HostConfig{}}, nil
}

func runBrowseCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdBrowse(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdBrowse(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		repo       string
		tty        bool
		wantURL    string
		wantOpened bool
		wantOut    string
	}{
		{
			name:       "repository",
			wantURL:    "https://app.copia.io/my-org/my-repo",
			wantOpened: true,
		},
		{
			name:       "issue or pull request number",
			args:       []string{"42"},
			wantURL:    "https://app.copia.io/my-org/my-repo/issues/42",
			wantOpened: true,
		},
		{
			name:       "number with a hash",
			args:       []string{"#42"},
			wantURL:    "https://app.copia.io/my-org/my-repo/issues/42",
			wantOpened: true,
		},
		{
			name:       "repo flag",
			repo:       "other-org/other-repo",
			args:       []string{"7"},
			wantURL:    "https://app.copia.io/other-org/other-repo/issues/7",
			wantOpened: true,
		},
		{
			name:       "repo flag path segments are escaped",
			repo:       "my org/my repo",
			wantURL:    "https://app.copia.io/my%20org/my%20repo",
			wantOpened: true,
		},
		{
			name:    "no-browser prints the repository URL",
			args:    []string{"--no-browser"},
			wantOut: "https://app.copia.io/my-org/my-repo\n",
		},
		{
			name:    "no-browser prints the issue URL",
			args:    []string{"42", "--no-browser"},
			repo:    "other-org/other-repo",
			wantOut: "https://app.copia.io/other-org/other-repo/issues/42\n",
		},
		{
			name:       "terminal names the URL being opened",
			args:       []string{"42"},
			tty:        true,
			wantURL:    "https://app.copia.io/my-org/my-repo/issues/42",
			wantOpened: true,
			wantOut:    "Opening app.copia.io/my-org/my-repo/issues/42 in your browser.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			t.Setenv("COPIA_HOST", "")

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			browser := &stubBrowser{}
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config:    noConfig,
				BaseRepo:  baseRepo,
				Browser:   browser,
				Host:      "app.copia.io",
				Repo:      tt.repo,
			}

			require.NoError(t, runBrowseCmd(f, tt.args...))

			if tt.wantOpened {
				assert.Equal(t, []string{tt.wantURL}, browser.urls)
			} else {
				assert.Empty(t, browser.urls)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdBrowse_HostFromConfigWithoutToken(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{"copia.example.com:3000": {}}}, nil
		},
		BaseRepo: baseRepo,
		Browser:  &stubBrowser{},
	}

	require.NoError(t, runBrowseCmd(f, "--no-browser"))
	assert.Equal(t, "https://copia.example.com:3000/my-org/my-repo\n", stdout.String())
}

func TestNewCmdBrowse_Errors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		host     string
		repo     string
		baseRepo func() (string, string, error)
		browser  *stubBrowser
		wantErr  string
	}{
		{
			name:    "not a number",
			args:    []string{"main.st"},
			host:    "app.copia.io",
			wantErr: `invalid argument "main.st": expected an issue or pull request number`,
		},
		{
			name:    "zero",
			args:    []string{"0"},
			host:    "app.copia.io",
			wantErr: `invalid argument "0": expected an issue or pull request number`,
		},
		{
			name:    "too many arguments",
			args:    []string{"1", "2"},
			host:    "app.copia.io",
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name:    "no host",
			wantErr: "no host configured. Run 'copia auth login' first",
		},
		{
			name:     "no repository",
			host:     "app.copia.io",
			baseRepo: func() (string, string, error) { return "", "", errors.New("not in a git repository") },
			wantErr:  "could not determine repository: not in a git repository",
		},
		{
			name:    "invalid repo flag",
			host:    "app.copia.io",
			repo:    "my-repo",
			wantErr: `could not determine repository: expected owner/repo format, got "my-repo"`,
		},
		{
			name:    "browser fails",
			host:    "app.copia.io",
			browser: &stubBrowser{err: errors.New("opening the browser: not found")},
			wantErr: "opening the browser: not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_HOST", "")
			ios, _, _, _ := iostreams.Test()
			browser := tt.browser
			if browser == nil {
				browser = &stubBrowser{}
			}
			br := tt.baseRepo
			if br == nil {
				br = baseRepo
			}
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config:    noConfig,
				BaseRepo:  br,
				Browser:   browser,
				Host:      tt.host,
				Repo:      tt.repo,
			}

			assert.EqualError(t, runBrowseCmd(f, tt.args...), tt.wantErr)
		})
	}
}
