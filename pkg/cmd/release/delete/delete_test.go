package delete

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

func TestDeleteRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"),
		httpmock.StringResponse(http.StatusOK, `{"id":42,"tag_name":"v1.0.0"}`),
	)
	reg.Register(
		httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo/releases/42"),
		httpmock.StringResponse(http.StatusNoContent, ``),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &DeleteOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
	}

	err := DeleteRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Deleted release v1.0.0")
}

func TestDeleteRun_NotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/v9.9.9"),
		httpmock.StringResponse(http.StatusNotFound, `{}`),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &DeleteOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v9.9.9",
	}

	err := DeleteRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func failingBody(status int) httpmock.Responder {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(errReader{}), Header: http.Header{}}, nil
	}
}

func transportError(*http.Request) (*http.Response, error) {
	return nil, errors.New("boom")
}

func TestDeleteRun_Errors(t *testing.T) {
	const lookupPath = "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"
	const deletePath = "/api/v1/repos/my-org/my-repo/releases/42"

	tests := []struct {
		name    string
		host    string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name:    "invalid host",
			host:    "bad\x7fhost",
			wantErr: "invalid control character",
		},
		{
			name: "lookup transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), transportError)
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name: "lookup server error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusInternalServerError, `{}`))
			},
			wantErr: "API error (HTTP 500)",
		},
		{
			name: "lookup body read error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), failingBody(http.StatusOK))
			},
			wantErr: "read failed",
		},
		{
			name: "lookup malformed JSON",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{not json`))
			},
			wantErr: "invalid character",
		},
		{
			name: "delete transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))
				reg.Register(httpmock.REST("DELETE", deletePath), transportError)
			},
			wantErr: "boom",
		},
		{
			name: "delete forbidden",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))
				reg.Register(httpmock.REST("DELETE", deletePath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to delete release (HTTP 403)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.stubs != nil {
				tt.stubs(reg)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			opts := &DeleteOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Tag:        "v1.0.0",
			}

			err := DeleteRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdDelete_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"),
		httpmock.StringResponse(http.StatusOK, `{"id":42,"tag_name":"v1.0.0"}`),
	)
	reg.Register(
		httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo/releases/42"),
		httpmock.StringResponse(http.StatusNoContent, ``),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"v1.0.0"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Deleted release v1.0.0\n", stdout.String())
	for _, req := range reg.Requests {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
	}
}

func TestNewCmdDelete_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name:    "missing tag",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "too many args",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"v1", "v2"},
			wantErr: "accepts 1 arg(s), received 2",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"v1.0.0"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"v1.0.0"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdDelete(tt.factory)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}
