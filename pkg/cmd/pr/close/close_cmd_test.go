package close

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

func errResponder(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("boom")
}

func runClose(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdClose(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdClose_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues/7/comments"),
		httpmock.StringResponse(http.StatusCreated, `{"id":1}`))
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusOK, `{"number":7,"head":{"label":"feature/x"}}`))
	reg.Register(httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo/branches/feature/x"),
		httpmock.StringResponse(http.StatusNoContent, ``))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	err := runClose(t, f, "7", "--comment", "bye", "--delete-branch")
	require.NoError(t, err)
	assert.Equal(t, "Closed PR #7\nDeleted branch feature/x\n", stdout.String())
	require.Len(t, reg.Requests, 3)
	assert.Equal(t, "token test-token", reg.Requests[1].Header.Get("Authorization"))
}

func TestNewCmdClose_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name: "invalid number",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios}
			},
			args:    []string{"abc"},
			wantErr: "invalid PR number: abc",
		},
		{
			name: "auth failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) {
					return nil, errors.New("no config")
				}}
			},
			args:    []string{"7"},
			wantErr: "no config",
		},
		{
			name: "repo failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			args:    []string{"7"},
			wantErr: "could not determine repository",
		},
		{
			name: "missing arg",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios}
			},
			args:    []string{},
			wantErr: "accepts 1 arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runClose(t, tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCloseRun_Errors(t *testing.T) {
	const commentPath = "/api/v1/repos/my-org/my-repo/issues/7/comments"
	const prPath = "/api/v1/repos/my-org/my-repo/pulls/7"

	tests := []struct {
		name    string
		host    string
		comment string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name:    "comment bad url",
			host:    "bad host",
			comment: "bye",
			stubs:   func(reg *httpmock.Registry) {},
			wantErr: "invalid character",
		},
		{
			name:    "comment transport error",
			comment: "bye",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", commentPath), errResponder)
			},
			wantErr: "boom",
		},
		{
			name:    "comment non-201",
			comment: "bye",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", commentPath),
					httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to add comment (HTTP 403)",
		},
		{
			name:    "close bad url",
			host:    "bad host",
			stubs:   func(reg *httpmock.Registry) {},
			wantErr: "invalid character",
		},
		{
			name: "close transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", prPath), errResponder)
			},
			wantErr: "boom",
		},
		{
			name: "close non-2xx",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", prPath),
					httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "failed to close PR (HTTP 404)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)

			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			ios, _, stdout, _ := iostreams.Test()
			err := CloseRun(&CloseOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Number:     7,
				Comment:    tt.comment,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestCloseRun_Created(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusCreated, `{"number":7}`))

	ios, _, stdout, _ := iostreams.Test()
	err := CloseRun(&CloseOptions{
		IO: ios, HTTPClient: &http.Client{Transport: reg},
		Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo", Number: 7,
	})
	require.NoError(t, err)
	assert.Equal(t, "Closed PR #7\n", stdout.String())
}

func TestCloseRun_DeleteBranch(t *testing.T) {
	const prPath = "/api/v1/repos/my-org/my-repo/pulls/7"

	tests := []struct {
		name       string
		prBody     string
		stubs      func(reg *httpmock.Registry)
		wantErr    string
		wantStdout string
	}{
		{
			name:       "no head label skips delete",
			prBody:     `{"number":7}`,
			stubs:      func(reg *httpmock.Registry) {},
			wantStdout: "Closed PR #7\n",
		},
		{
			name:   "delete returns 200",
			prBody: `{"head":{"label":"feat"}}`,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "/branches/feat"),
					httpmock.StringResponse(http.StatusOK, ``))
			},
			wantStdout: "Closed PR #7\nDeleted branch feat\n",
		},
		{
			name:   "delete non-success is silent",
			prBody: `{"head":{"label":"feat"}}`,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "/branches/feat"),
					httpmock.StringResponse(http.StatusNotFound, ``))
			},
			wantStdout: "Closed PR #7\n",
		},
		{
			name:   "delete transport error",
			prBody: `{"head":{"label":"feat"}}`,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "/branches/feat"), errResponder)
			},
			wantErr:    "failed to delete branch: ",
			wantStdout: "Closed PR #7\n",
		},
		{
			name:       "delete bad url",
			prBody:     `{"head":{"label":"bad%zz"}}`,
			stubs:      func(reg *httpmock.Registry) {},
			wantErr:    "invalid URL escape",
			wantStdout: "Closed PR #7\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("PATCH", prPath),
				httpmock.StringResponse(http.StatusOK, tt.prBody))
			tt.stubs(reg)

			ios, _, stdout, _ := iostreams.Test()
			err := CloseRun(&CloseOptions{
				IO: ios, HTTPClient: &http.Client{Transport: reg},
				Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo",
				Number: 7, DeleteBranch: true,
			})
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
