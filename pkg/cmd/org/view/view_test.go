package view

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

func TestViewRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/orgs/my-org"),
		httpmock.StringResponse(http.StatusOK, `{"username":"my-org","full_name":"My Organization","description":"Industrial automation","website":"https://example.com"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Name:       "my-org",
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "my-org")
	assert.Contains(t, stdout.String(), "Industrial automation")
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errBody) Close() error             { return nil }

func runViewCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdView(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdView_RunE_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org"),
		httpmock.StringResponse(http.StatusOK, `{"username":"my-org","full_name":"My Org","description":"d","website":"https://example.com"}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runViewCmd(t, f, "my-org", "--json", "username,website")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"username": "my-org"`)
	assert.Contains(t, stdout.String(), `"website": "https://example.com"`)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdView_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runViewCmd(t, f, "my-org")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}

func TestNewCmdView_RequiresOneArg(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runViewCmd(t, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg(s)")
}

func TestViewRun_MinimalFields(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/bare"),
		httpmock.StringResponse(http.StatusOK, `{"username":"bare","full_name":""}`))

	ios, _, stdout, _ := iostreams.Test()
	err := ViewRun(&ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Name:       "bare",
	})
	require.NoError(t, err)
	assert.Equal(t, "bare ()\n", stdout.String())
}

func TestViewRun_Errors(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		responder httpmock.Responder
		wantErr   string
	}{
		{
			name:    "invalid host",
			host:    "bad host",
			wantErr: "invalid character",
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io",
		},
		{
			name:      "not found",
			responder: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr:   "organization my-org not found",
		},
		{
			name:      "server error",
			responder: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr:   "API error (HTTP 500)",
		},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: errBody{}}, nil
			},
			wantErr: "read failed",
		},
		{
			name:      "malformed json",
			responder: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr:   "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org"), tt.responder)
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
				Name:       "my-org",
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
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org"),
		httpmock.StringResponse(http.StatusOK, `{"username":"my-org","full_name":"My Org","description":"d","website":"https://example.com"}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	require.NoError(t, runViewCmd(t, f, "my-org", "--json", "username,website", "--jq", "keys"))
	assert.Equal(t, "[\"username\",\"website\"]\n", stdout.String())
}
