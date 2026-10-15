package fork

import (
	"encoding/json"
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
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var sent forkRequest
	reg.Register(httpmock.REST("POST", "/api/v1/repos/upstream-org/project/forks"),
		func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&sent); err != nil {
				return nil, err
			}
			return httpmock.StringResponse(http.StatusCreated,
				`{"full_name":"my-org/project","html_url":"https://app.copia.io/my-org/project"}`)(req)
		})

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	require.NoError(t, runForkCmd(f, "upstream-org/project", "--org", "my-org"))
	assert.Equal(t, "my-org", sent.Organization)
	assert.Equal(t, "Forked to my-org/project\nhttps://app.copia.io/my-org/project\n", stdout.String())
}

func TestNewCmdFork_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

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
			name: "invalid repo",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			args:    []string{"project"},
			wantErr: "expected owner/repo format",
		},
		{
			name: "missing arg",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			wantErr: "accepts 1 arg",
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

func TestForkRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "not found",
			respond: httpmock.StringResponse(http.StatusNotFound, `{"message":"not found"}`),
			wantErr: "failed to fork repository (HTTP 404): {\"message\":\"not found\"}",
		},
		{
			name:    "malformed json",
			respond: httpmock.StringResponse(http.StatusAccepted, `{not json`),
			wantErr: "invalid character",
		},
		{
			name: "body read error",
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(errReader{})}, nil
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
				reg.Register(httpmock.REST("POST", "/api/v1/repos/upstream-org/project/forks"), tt.respond)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := ForkRun(&ForkOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "upstream-org",
				Repo:       "project",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
