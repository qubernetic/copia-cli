package fork

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func runForkCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdFork(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdFork_Run(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		repo         string // -R
		baseRepo     bool
		wantSent     forkRequest
		wantForkPath string
	}{
		{
			name:     "repository argument",
			args:     []string{"upstream-org/project", "--org", "my-org", "--fork-name", "renamed"},
			wantSent: forkRequest{Organization: "my-org", Name: "renamed"},
		},
		{
			name:     "current repository",
			baseRepo: true,
		},
		{
			name: "repository flag",
			repo: "upstream-org/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			var sent forkRequest
			reg.Register(httpmock.REST("POST", "/api/v1/repos/upstream-org/project/forks"),
				decodeInto(&sent, httpmock.StringResponse(http.StatusAccepted, orgFork)))

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: tt.repo}
			if tt.baseRepo {
				f.BaseRepo = func() (string, string, error) { return "upstream-org", "project", nil }
			}

			require.NoError(t, runForkCmd(f, tt.args...))
			assert.Equal(t, tt.wantSent, sent)
			assert.Equal(t, "https://app.copia.io/my-org/project\n", stdout.String())
		})
	}
}

func TestNewCmdFork_Errors(t *testing.T) {
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
			args:    []string{"upstream-org/project"},
			wantErr: "no config",
		},
		{
			name:    "invalid repo",
			factory: authed,
			args:    []string{"project"},
			wantErr: "argument error: expected owner/repo format",
		},
		{
			name:    "no current repository",
			factory: authed,
			wantErr: "unable to determine base repository: could not determine repository",
		},
		{
			name: "invalid repository flag",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				f := authed(ios)
				f.Repo = "project"
				return f
			},
			wantErr: "unable to determine base repository: expected owner/repo format",
		},
		{
			name:    "too many args",
			factory: authed,
			args:    []string{"a/b", "c/d"},
			wantErr: "accepts at most 1 arg",
		},
		{
			name:    "blank org",
			factory: authed,
			args:    []string{"upstream-org/project", "--org="},
			wantErr: "--org cannot be blank",
		},
		{
			name:    "blank remote name",
			factory: authed,
			args:    []string{"--remote", "--remote-name="},
			wantErr: "--remote-name cannot be blank",
		},
		{
			name:    "remote with a repository argument",
			factory: authed,
			args:    []string{"upstream-org/project", "--remote"},
			wantErr: "the `--remote` flag is unsupported when a repository argument is provided",
		},
		{
			name:    "remote=false with a repository argument",
			factory: authed,
			args:    []string{"upstream-org/project", "--remote=false"},
			wantErr: "the `--remote` flag is unsupported when a repository argument is provided",
		},
		{
			name: "remote with the repository flag",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				f := authed(ios)
				f.Repo = "upstream-org/project"
				return f
			},
			args:    []string{"--remote"},
			wantErr: "the `--remote` flag is unsupported when a repository argument is provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runForkCmd(tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
