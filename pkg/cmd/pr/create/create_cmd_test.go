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

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func runCreate(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdCreate(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdCreate_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls"),
		httpmock.StringResponse(http.StatusCreated,
			`{"number":8,"title":"feat: x","html_url":"https://app.copia.io/my-org/my-repo/pulls/8"}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	err := runCreate(t, f, "--title", "feat: x", "--body", "desc", "--base", "develop", "--head", "feature/x")
	require.NoError(t, err)
	assert.Equal(t, "Created PR #8: feat: x\nhttps://app.copia.io/my-org/my-repo/pulls/8\n", stdout.String())

	require.Len(t, reg.Requests, 1)
	var sent map[string]string
	require.NoError(t, json.NewDecoder(reg.Requests[0].Body).Decode(&sent))
	assert.Equal(t, map[string]string{"title": "feat: x", "body": "desc", "base": "develop", "head": "feature/x"}, sent)
}

func TestNewCmdCreate_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		wantErr string
	}{
		{
			name: "auth failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) {
					return nil, errors.New("no config")
				}}
			},
			wantErr: "no config",
		},
		{
			name: "repo failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			wantErr: "could not determine repository",
		},
		{
			name: "missing title",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			},
			wantErr: "title required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runCreate(t, tt.factory(ios), "--head", "feature/x")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCreateRun_Errors(t *testing.T) {
	const path = "/api/v1/repos/my-org/my-repo/pulls"

	tests := []struct {
		name      string
		host      string
		responder httpmock.Responder
		wantErr   string
	}{
		{
			name:    "bad url",
			host:    "bad host",
			wantErr: "invalid character",
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:      "non-201",
			responder: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"exists"}`),
			wantErr:   `failed to create PR (HTTP 422): {"message":"exists"}`,
		},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(failingReader{})}, nil
			},
			wantErr: "read failed",
		},
		{
			name:      "malformed json",
			responder: httpmock.StringResponse(http.StatusCreated, `{not json`),
			wantErr:   "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("POST", path), tt.responder)
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
				Owner:      "my-org",
				Repo:       "my-repo",
				Title:      "feat: x",
				Base:       "main",
				Head:       "feature/x",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
