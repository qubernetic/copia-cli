package list

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

func runListCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdList(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdList_Run(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		path  string
		check func(t *testing.T, out string)
	}{
		{
			name: "user table",
			path: "/api/v1/user/repos",
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "john/plc-project")
				assert.Contains(t, out, "private")
			},
		},
		{
			name: "org json",
			args: []string{"--org", "my-org", "-L", "5", "--json", "full_name"},
			path: "/api/v1/orgs/my-org/repos",
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"full_name": "john/plc-project"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", tt.path),
				httpmock.StringResponse(http.StatusOK, `[{"full_name":"john/plc-project","description":"PLC","private":true}]`))

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

			require.NoError(t, runListCmd(f, tt.args...))
			tt.check(t, stdout.String())
		})
	}
}

func TestNewCmdList_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}

	err := runListCmd(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}

func TestListRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "server error",
			respond: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "HTTP 500",
		},
		{
			name:    "malformed json",
			respond: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr: "parsing response",
		},
		{
			name: "transport error",
			respond: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/user/repos"), tt.respond)

			ios, _, _, _ := iostreams.Test()
			err := ListRun(&ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Limit:      30,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
