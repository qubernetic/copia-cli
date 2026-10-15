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

func TestCreateRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusCreated, `{"id":1,"tag_name":"v1.0.0","name":"Release 1.0.0","html_url":"https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &CreateOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
		Title:      "Release 1.0.0",
		Notes:      "First stable release.",
	}

	err := CreateRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "v1.0.0")
}

func TestCreateRun_MissingTag(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &CreateOptions{IO: ios, Tag: ""}

	err := CreateRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag required")
}

func TestCreateRun_Draft(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusCreated, `{"id":2,"tag_name":"v2.0.0-rc.1","name":"RC1","draft":true,"html_url":"https://app.copia.io/my-org/my-repo/releases/tag/v2.0.0-rc.1"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &CreateOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v2.0.0-rc.1",
		Title:      "RC1",
		Draft:      true,
	}

	err := CreateRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "v2.0.0-rc.1")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestCreateRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "invalid host",
			host:    "bad\x7fhost",
			wantErr: "invalid control character",
		},
		{
			name: "transport error",
			respond: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:    "conflict",
			respond: httpmock.StringResponse(http.StatusConflict, `{"message":"tag exists"}`),
			wantErr: `failed to create release (HTTP 409): {"message":"tag exists"}`,
		},
		{
			name: "body read error",
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(errReader{}), Header: http.Header{}}, nil
			},
			wantErr: "read failed",
		},
		{
			name:    "malformed JSON",
			respond: httpmock.StringResponse(http.StatusCreated, `{not json`),
			wantErr: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.respond != nil {
				reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/releases"), tt.respond)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			opts := &CreateOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Tag:        "v1.0.0",
			}

			err := CreateRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdCreate_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var payload map[string]interface{}
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/releases"),
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &payload))
			return httpmock.StringResponse(http.StatusCreated,
				`{"id":3,"tag_name":"v2.0.0-rc.1","name":"RC1","html_url":"https://app.copia.io/my-org/my-repo/releases/tag/v2.0.0-rc.1"}`)(req)
		},
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdCreate(f)
	cmd.SetArgs([]string{"v2.0.0-rc.1", "-t", "RC1", "-n", "Notes", "--draft", "--prerelease"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, map[string]interface{}{
		"tag_name":   "v2.0.0-rc.1",
		"name":       "RC1",
		"body":       "Notes",
		"draft":      true,
		"prerelease": true,
	}, payload)
	assert.Equal(t, "Created release v2.0.0-rc.1: RC1\nhttps://app.copia.io/my-org/my-repo/releases/tag/v2.0.0-rc.1\n", stdout.String())
}

func TestNewCmdCreate_RunEErrors(t *testing.T) {
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
			name:    "unknown flag",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"v1.0.0", "--bogus"},
			wantErr: "unknown flag: --bogus",
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
			cmd := NewCmdCreate(tt.factory)
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
