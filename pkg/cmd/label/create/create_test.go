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
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusCreated, `{"id":1,"name":"critical","color":"#e11d48"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &CreateOptions{
		IO:          ios,
		HTTPClient:  &http.Client{Transport: reg},
		Host:        "app.copia.io",
		Token:       "test-token",
		Owner:       "my-org",
		Repo:        "my-repo",
		Name:        "critical",
		Color:       "#e11d48",
		Description: "Critical issue",
	}

	err := CreateRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "critical")
}

func TestCreateRun_MissingName(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &CreateOptions{
		IO:   ios,
		Name: "",
	}

	err := CreateRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name required")
}

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
			name:    "validation error",
			respond: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"invalid color"}`),
			wantErr: "failed to create label (HTTP 422)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.respond != nil {
				reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/labels"), tt.respond)
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
				Name:       "bug",
				Color:      "zzz",
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
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/labels"),
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &payload))
			return httpmock.StringResponse(http.StatusCreated, `{"id":1}`)(req)
		},
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdCreate(f)
	cmd.SetArgs([]string{"-n", "feature", "-c", "#0969da", "-d", "New feature"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, map[string]interface{}{
		"name":        "feature",
		"color":       "#0969da",
		"description": "New feature",
	}, payload)
	assert.Equal(t, "Label \"feature\" created\n", stdout.String())
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
			name:    "missing name",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--color", "#000000"},
			wantErr: "name required",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"--name", "bug"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"--name", "bug"},
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
