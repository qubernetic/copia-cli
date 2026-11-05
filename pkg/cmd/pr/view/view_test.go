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
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusOK, `{
			"number":7,"title":"feat: add cylinder wrapper","state":"open",
			"body":"Adds cylinder control.","mergeable":true,
			"html_url":"https://app.copia.io/my-org/my-repo/pulls/7",
			"user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feature/cylinder"},
			"created_at":"2026-03-30T10:00:00Z"
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
		Number:     7,
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "feat: add cylinder wrapper")
	assert.Contains(t, stdout.String(), "john")
	assert.Contains(t, stdout.String(), "main <- feature/cylinder")
	assert.NotContains(t, stdout.String(), "Labels:", "no labels line without labels")
}

func TestViewRun_Labels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusOK, `{
			"number":7,"title":"feat: test","state":"open","user":{"login":"john"},
			"base":{"label":"main"},"head":{"label":"feat"},
			"labels":[{"id":1,"name":"plc"},{"id":2,"name":"urgent"}]
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	err := ViewRun(&ViewOptions{
		IO: ios, HTTPClient: &http.Client{Transport: reg},
		Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo", Number: 7,
	})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Branches: main <- feat\nLabels: plc, urgent\n")
}

func TestViewRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls/7"),
		httpmock.StringResponse(http.StatusOK, `{
			"number":7,"title":"feat: test","state":"open","body":"","mergeable":true,
			"html_url":"","user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feat"},
			"created_at":"2026-03-30T10:00:00Z"
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
		Number:     7,
		JSON:       cmdutil.JSONFlags{Fields: []string{"number", "title"}},
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"number"`)
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
