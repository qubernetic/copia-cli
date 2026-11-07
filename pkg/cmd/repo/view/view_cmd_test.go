package view

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

const privateRepoJSON = `{"full_name":"my-org/my-repo","html_url":"https://app.copia.io/my-org/my-repo",
	"private":true,"default_branch":"main","stars_count":1,"forks_count":2,"open_issues_count":3}`

func runViewCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdView(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdView_Run(t *testing.T) {
	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		args    []string
	}{
		{
			name: "positional repo",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			args: []string{"my-org/my-repo"},
		},
		{
			name: "repo flag",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			},
		},
		{
			name: "base repo",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{
					IOStreams: ios, Host: "app.copia.io", Token: "test-token",
					BaseRepo: func() (string, string, error) { return "my-org", "my-repo", nil },
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"),
				httpmock.StringResponse(http.StatusOK, privateRepoJSON))

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runViewCmd(tt.factory(ios), tt.args...))
			assert.Equal(t, "my-org/my-repo (private)\n\nDefault branch: main\n"+
				"Stars: 1  Forks: 2  Open issues: 3\nURL: https://app.copia.io/my-org/my-repo\n", stdout.String())
		})
	}
}

func TestNewCmdView_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	authed := func(ios *iostreams.IOStreams) *cmdutil.Factory {
		return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	}

	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name: "auth failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{
					IOStreams: ios,
					Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
				}
			},
			args:    []string{"my-org/my-repo"},
			wantErr: "no config",
		},
		{
			name:    "invalid repo arg",
			factory: authed,
			args:    []string{"my-repo"},
			wantErr: "expected owner/repo format",
		},
		{
			name:    "no repo",
			factory: authed,
			wantErr: "could not determine repository: could not determine repository",
		},
		{
			name:    "too many args",
			factory: authed,
			args:    []string{"a/b", "c/d"},
			wantErr: "accepts at most 1 arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runViewCmd(tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestViewRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "server error",
			respond: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "API error (HTTP 500)",
		},
		{
			name:    "malformed json",
			respond: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr: "parsing response",
		},
		{
			name: "body read error",
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{})}, nil
			},
			wantErr: "read failed",
		},
		{
			name: "transport error",
			respond: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io",
		},
		{
			name:    "invalid host",
			host:    "bad host",
			wantErr: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.respond != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"), tt.respond)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := ViewRun(&ViewOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdView_JSONFieldsAndJQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"), httpmock.StringResponse(http.StatusOK, privateRepoJSON))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runViewCmd(f, "--json", "stars_count,forks_count,html_url", "--jq", "[.stars_count, .forks_count, .html_url]"))
	assert.Equal(t, "[1,2,\"https://app.copia.io/my-org/my-repo\"]\n", stdout.String())
}

func TestNewCmdView_Web(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "current repo", args: []string{"--web"}, repo: "my-org/my-repo", wantURL: "https://app.copia.io/my-org/my-repo"},
		{name: "short flag", args: []string{"-w"}, repo: "my-org/my-repo", wantURL: "https://app.copia.io/my-org/my-repo"},
		{name: "positional repo", args: []string{"other-org/other-repo", "-w"}, wantURL: "https://app.copia.io/other-org/other-repo"},
		{
			name: "tty", args: []string{"-w"}, repo: "my-org/my-repo", tty: true,
			wantURL: "https://app.copia.io/my-org/my-repo",
			wantErr: "Opening app.copia.io/my-org/my-repo in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: tt.repo, Browser: b}

			require.NoError(t, runViewCmd(f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdView_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}

	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: b,
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runViewCmd(noConfig, "-w"), "no config")

	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}
	require.Error(t, runViewCmd(f, "not-a-repo", "-w"))

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: b}
	err := runViewCmd(noRepo, "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
	assert.Empty(t, b.URLs)

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runViewCmd(failing, "-w"), "no browser")
	assert.Empty(t, reg.Requests)
}

func TestNewCmdView_JSONArchived(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"), httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo","archived":true}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runViewCmd(f, "--json", "archived"))
	assert.JSONEq(t, `{"archived":true}`, stdout.String())
}
