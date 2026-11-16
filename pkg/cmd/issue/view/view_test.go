package view

import (
	"context"
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
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/issues/12"),
		httpmock.StringResponse(http.StatusOK, `{
			"number":12,"title":"Fix PLC connection timeout",
			"body":"The PLC connection times out.",
			"state":"open","html_url":"https://app.copia.io/my-org/my-repo/issues/12",
			"user":{"login":"john"},"labels":[{"name":"bug"}],
			"created_at":"2026-03-30T10:00:00Z","comments":2
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Fix PLC connection timeout")
	assert.Contains(t, stdout.String(), "john")
}

func TestViewRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/issues/12"),
		httpmock.StringResponse(http.StatusOK, `{
			"number":12,"title":"Fix PLC","body":"","state":"open",
			"html_url":"","user":{"login":"john"},"labels":[],"created_at":"2026-03-30T10:00:00Z","comments":0
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		JSON:       cmdutil.JSONFlags{Fields: []string{"number", "title"}},
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"number"`)
}

func TestViewRun_NotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/issues/999"),
		httpmock.StringResponse(http.StatusNotFound, `{}`),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     999,
	}

	err := ViewRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNewCmdView_CommentsWithDefaultJSON(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "comments drop the json default", args: []string{"--comments"}, wantErr: "no host configured. Run 'copia auth login' first"},
		{name: "comments with explicit json", args: []string{"--comments", "--json", "number"}, wantErr: "specify only one of --comments or --json"},
		{name: "comments with jq", args: []string{"--comments", "--jq", "."}, wantErr: "specify only one of --comments or --json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config:    func() (*config.Config, error) { return &config.Config{}, nil },
			}
			cmd := NewCmdView(f)
			cmd.SetArgs(append([]string{"1"}, tt.args...))
			cmd.SetOut(ios.Out)
			cmd.SetErr(ios.ErrOut)
			jsonFormat := func() (string, error) { return cmdutil.OutputJSON, nil }
			err := cmd.ExecuteContext(cmdutil.WithOutputFormat(context.Background(), jsonFormat))
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
