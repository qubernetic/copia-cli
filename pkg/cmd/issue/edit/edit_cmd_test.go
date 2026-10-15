package edit

import (
	"bytes"
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

const (
	issuePath       = "/api/v1/repos/my-org/my-repo/issues/12"
	issueLabelsPath = "/api/v1/repos/my-org/my-repo/issues/12/labels"
	repoLabelsPath  = "/api/v1/repos/my-org/my-repo/labels"
	orgLabelsPath   = "/api/v1/orgs/my-org/labels"
)

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdEdit(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func failing(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

func TestNewCmdEdit_RunsAllFlags(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", orgLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":90,"name":"urgent"}]`))
	var labelsPosted, patched string
	reg.Register(httpmock.REST("POST", issueLabelsPath), recordBody(&labelsPosted, httpmock.StringResponse(http.StatusCreated, `[]`)))
	reg.Register(httpmock.REST("PATCH", issuePath), recordBody(&patched, httpmock.StringResponse(http.StatusCreated, `{}`)))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12",
		"-t", "New title", "-b", "New body",
		"--add-label", "bug", "--add-label", "urgent",
		"-a", "john,jane", "-m", "3")
	require.NoError(t, err)
	assert.JSONEq(t, `{"labels":[1,90]}`, labelsPosted)
	assert.JSONEq(t, `{"title":"New title","body":"New body","assignees":["john","jane"],"milestone":3}`, patched)
	assert.Equal(t, "Updated issue #12\n", stdout.String())
}

func TestNewCmdEdit_InvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "abc", "--title", "x")
	require.EqualError(t, err, "invalid issue number: abc")
}

func TestNewCmdEdit_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runCmd(t, f, "12", "--title", "x")
	require.EqualError(t, err, "no config")
}

func TestNewCmdEdit_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12", "--title", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

func TestNewCmdEdit_NothingToEdit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to edit")
}

func TestNewCmdEdit_InvalidMilestoneFlag(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--milestone", "abc")
	require.Error(t, err)
}

func editOpts(reg *httpmock.Registry) (*EditOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
	}, stdout
}

func registerLabelLookup(reg *httpmock.Registry) {
	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", orgLabelsPath), httpmock.StringResponse(http.StatusOK, `[]`))
}

func TestEditRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		labels  []string
		title   string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name:   "add labels rejected",
			labels: []string{"bug"},
			title:  "x",
			stubs: func(reg *httpmock.Registry) {
				registerLabelLookup(reg)
				reg.Register(httpmock.REST("POST", issueLabelsPath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to add labels (HTTP 403)",
		},
		{
			name:   "add labels transport error",
			labels: []string{"bug"},
			stubs: func(reg *httpmock.Registry) {
				registerLabelLookup(reg)
				reg.Register(httpmock.REST("POST", issueLabelsPath), failing)
			},
			wantErr: "boom",
		},
		{
			name:  "update rejected",
			title: "x",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "failed to update issue (HTTP 404)",
		},
		{
			name:  "update transport error",
			title: "x",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", issuePath), failing)
			},
			wantErr: "boom",
		},
		{
			name:   "labels ok then update rejected",
			labels: []string{"bug"},
			title:  "x",
			stubs: func(reg *httpmock.Registry) {
				registerLabelLookup(reg)
				reg.Register(httpmock.REST("POST", issueLabelsPath), httpmock.StringResponse(http.StatusOK, `[]`))
				reg.Register(httpmock.REST("PATCH", issuePath), httpmock.StringResponse(http.StatusInternalServerError, `{}`))
			},
			wantErr: "failed to update issue (HTTP 500)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)

			opts, stdout := editOpts(reg)
			opts.AddLabels = tt.labels
			opts.Title = tt.title
			err := EditRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestEditRun_SetBodyOnly(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	var patched string
	reg.Register(httpmock.REST("PATCH", issuePath), recordBody(&patched, httpmock.StringResponse(http.StatusOK, `{}`)))

	opts, stdout := editOpts(reg)
	opts.Body = "only body"
	require.NoError(t, EditRun(opts))
	assert.JSONEq(t, `{"body":"only body"}`, patched)
	assert.Equal(t, "Updated issue #12\n", stdout.String())
}

func TestEditRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _ := editOpts(reg)
	opts.Host = "bad\x7fhost"
	opts.Title = "x"
	require.Error(t, EditRun(opts))
	assert.Empty(t, reg.Requests)
}
