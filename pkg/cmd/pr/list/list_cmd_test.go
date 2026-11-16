package list

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runList(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdList(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdList_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/my-repo/pulls",
			url.Values{"state": {"all"}, "page": {"1"}, "limit": {"5"}}),
		httpmock.StringResponse(http.StatusOK,
			`[{"number":3,"title":"t","state":"closed","user":{"login":"amy"},"base":{"label":"main"},"head":{"label":"f"}}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	require.NoError(t, runList(t, f, "--state", "all", "--limit", "5"))
	assert.Equal(t, "#3  t  closed  amy  main <- f\n", stdout.String())
}

func TestNewCmdList_JSONFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls"),
		httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	require.NoError(t, runList(t, f, "--json", "number"))
	assert.Equal(t, "[]\n", stdout.String())
}

func TestNewCmdList_Errors(t *testing.T) {
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
			name: "invalid limit",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			},
			args:    []string{"--limit", "0"},
			wantErr: "invalid limit: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runList(t, tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestListRun_FetchErrors(t *testing.T) {
	tests := []struct {
		name      string
		responder httpmock.Responder
		wantErr   string
	}{
		{
			name:      "server error",
			responder: httpmock.StringResponse(http.StatusInternalServerError, ``),
			wantErr:   "API error (HTTP 500)",
		},
		{
			name:      "malformed json",
			responder: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr:   "parsing response",
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls"), tt.responder)

			ios, _, stdout, _ := iostreams.Test()
			err := ListRun(&ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				State:      "merged",
				Limit:      30,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdList_JSONFieldsAndJQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls"), httpmock.StringResponse(http.StatusOK,
		`[{"number":3,"title":"t","state":"open","user":{"login":"amy"},"base":{"label":"main"},"head":{"label":"f"}}]`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runList(t, f, "--json", "number,user", "--jq", ".[] | [.number, .user.login]"))
	assert.Equal(t, "[3,\"amy\"]\n", stdout.String())
}
