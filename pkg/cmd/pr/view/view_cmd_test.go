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

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func runView(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdView(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdView_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusOK, `{
			"number":7,"title":"feat: x","state":"open","body":"","mergeable":false,
			"html_url":"https://app.copia.io/my-org/my-repo/pulls/7",
			"user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feat"}
		}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	require.NoError(t, runView(t, f, "7"))
	assert.Equal(t, "#7 feat: x\n"+
		"State: open  Author: john  Mergeable: false\n"+
		"Branches: main <- feat\n"+
		"\nhttps://app.copia.io/my-org/my-repo/pulls/7\n", stdout.String())
}

func TestNewCmdView_JSONFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusOK, `{"number":7,"title":"feat: x"}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	require.NoError(t, runView(t, f, "7", "--json", "number,title"))
	assert.Contains(t, stdout.String(), `"number": 7`)
	assert.Contains(t, stdout.String(), `"title": "feat: x"`)
}

func TestNewCmdView_Errors(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runView(t, tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestViewRun_Errors(t *testing.T) {
	const path = "/api/v1/repos/my-org/my-repo/pulls/7"

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
			name:      "not found",
			responder: httpmock.StringResponse(http.StatusNotFound, ``),
			wantErr:   "PR #7 not found",
		},
		{
			name:      "server error",
			responder: httpmock.StringResponse(http.StatusBadGateway, ``),
			wantErr:   "API error (HTTP 502)",
		},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}, nil
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
				reg.Register(httpmock.REST("GET", path), tt.responder)
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
				Number:     7,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

const (
	prPath       = "/api/v1/repos/my-org/my-repo/pulls/7"
	commentsPath = "/api/v1/repos/my-org/my-repo/issues/7/comments"
	prOutput     = "#7 feat: x\n" +
		"State: open  Author: john  Mergeable: true\n" +
		"Branches: main <- feat\n" +
		"\nAdds x\n"
	prURLOutput = "\nhttps://app.copia.io/my-org/my-repo/pulls/7\n"
)

func registerPRWithBody(reg *httpmock.Registry) {
	reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, `{
		"number":7,"title":"feat: x","state":"open","body":"Adds x","mergeable":true,
		"html_url":"https://app.copia.io/my-org/my-repo/pulls/7",
		"user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feat"}
	}`))
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdView_Comments(t *testing.T) {
	for _, flag := range []string{"--comments", "-c"} {
		t.Run(flag, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			registerPRWithBody(reg)
			reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusOK,
				`[{"body":"Please rebase","user":{"login":"jane"},"created_at":"2026-03-30T10:00:00Z"},`+
					`{"body":"Done","user":{"login":"john"},"created_at":"2026-03-31T09:30:00Z"}]`))

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runView(t, readyFactory(ios), "7", flag))
			assert.Equal(t, prOutput+
				"\njane commented 2026-03-30T10:00:00Z:\nPlease rebase\n"+
				"\njohn commented 2026-03-31T09:30:00Z:\nDone\n"+
				prURLOutput, stdout.String())
			require.Len(t, reg.Requests, 2)
			assert.Equal(t, "token test-token", reg.Requests[1].Header.Get("Authorization"))
		})
	}
}

func TestNewCmdView_NoCommentsRequestWithoutFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerPRWithBody(reg)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runView(t, readyFactory(ios), "7"))
	assert.Equal(t, prOutput+prURLOutput, stdout.String())
	assert.Len(t, reg.Requests, 1)
}

func TestNewCmdView_ZeroComments(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerPRWithBody(reg)
	reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runView(t, readyFactory(ios), "7", "--comments"))
	assert.Equal(t, prOutput+prURLOutput, stdout.String())
}

func TestNewCmdView_CommentsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerPRWithBody(reg)
	reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusBadGateway, ``))

	ios, _, stdout, _ := iostreams.Test()
	err := runView(t, readyFactory(ios), "7", "--comments")
	require.EqualError(t, err, "API error (HTTP 502)")
	assert.Empty(t, stdout.String())
}

func TestNewCmdView_CommentsWithJSON(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, _, _ := iostreams.Test()
	err := runView(t, readyFactory(ios), "7", "--comments", "--json", "number")
	require.EqualError(t, err, "specify only one of --comments or --json")
	assert.Empty(t, reg.Requests)
}

func TestNewCmdView_JSONFieldsAndJQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerPRWithBody(reg)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runView(t, readyFactory(ios), "7", "--json", "number,user,html_url"))
	assert.Equal(t, `{
  "html_url": "https://app.copia.io/my-org/my-repo/pulls/7",
  "number": 7,
  "user": {
    "login": "john"
  }
}
`, stdout.String())

	err := runView(t, readyFactory(ios), "7", "--json", "author")
	require.ErrorContains(t, err, "unknown JSON field: \"author\"")
}
