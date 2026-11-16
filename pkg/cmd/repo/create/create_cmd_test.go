package create

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

func runCreateCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdCreate(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdCreate_Run(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var sent createRequest
	reg.Register(httpmock.REST("POST", "/api/v1/orgs/my-org/repos"),
		func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&sent); err != nil {
				return nil, err
			}
			return httpmock.StringResponse(http.StatusCreated,
				`{"full_name":"my-org/new-repo","html_url":"https://app.copia.io/my-org/new-repo"}`)(req)
		})

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	err := runCreateCmd(f, "new-repo", "--org", "my-org", "--private", "-d", "PLC project")
	require.NoError(t, err)
	assert.Equal(t, createRequest{Name: "new-repo", Description: "PLC project", Private: true}, sent)
	assert.Equal(t, "Created repository my-org/new-repo\nhttps://app.copia.io/my-org/new-repo\n", stdout.String())
}

func TestNewCmdCreate_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	t.Run("auth failure", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		f := &cmdutil.Factory{
			IOStreams: ios,
			Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
		}
		err := runCreateCmd(f, "new-repo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no config")
	})

	t.Run("missing arg", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
		err := runCreateCmd(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})
}

func TestCreateRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "conflict",
			respond: httpmock.StringResponse(http.StatusConflict, `{"message":"already exists"}`),
			wantErr: "failed to create repository (HTTP 409): {\"message\":\"already exists\"}",
		},
		{
			name:    "malformed json",
			respond: httpmock.StringResponse(http.StatusCreated, `{not json`),
			wantErr: "invalid character",
		},
		{
			name: "body read error",
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(errReader{})}, nil
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
				reg.Register(httpmock.REST("POST", "/api/v1/user/repos"), tt.respond)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := CreateRun(&CreateOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Name:       "new-repo",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
