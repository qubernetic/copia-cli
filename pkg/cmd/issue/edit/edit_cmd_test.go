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
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/3"), httpmock.StringResponse(http.StatusOK, `{"id":3,"title":"v1.0"}`))
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

func TestNewCmdEdit_MilestoneByNameOrID(t *testing.T) {
	for _, milestone := range []string{"v1.0", "1"} {
		t.Run(milestone, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/"+milestone), httpmock.StringResponse(http.StatusOK, `{"id":1,"title":"v1.0"}`))
			var patched string
			reg.Register(httpmock.REST("PATCH", issuePath), recordBody(&patched, httpmock.StringResponse(http.StatusCreated, `{}`)))

			ios, _, _, _ := iostreams.Test()
			err := runCmd(t, readyFactory(ios), "12", "--milestone", milestone)
			require.NoError(t, err)
			assert.JSONEq(t, `{"milestone":1}`, patched)
		})
	}
}

func TestNewCmdEdit_UnknownMilestone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/abc"), httpmock.StringResponse(http.StatusNotFound, ``))

	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--milestone", "abc")
	require.EqualError(t, err, `milestone "abc" not found in my-org/my-repo`)
	assert.Len(t, reg.Requests, 1, "the issue is not patched")
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

func TestNewCmdEdit_AddAndRemoveAssignees(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{"number":12,"assignees":[{"login":"john"},{"login":"jane"}]}`))
	var patched string
	reg.Register(httpmock.REST("PATCH", issuePath), recordBody(&patched, httpmock.StringResponse(http.StatusCreated, `{}`)))

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "--add-assignee", "bob", "--remove-assignee", "john")
	require.NoError(t, err)
	assert.JSONEq(t, `{"assignees":["jane","bob"]}`, patched)
	assert.Equal(t, "Updated issue #12\n", stdout.String())
}

func TestNewCmdEdit_AssigneeConflicts(t *testing.T) {
	for _, flag := range []string{"--add-assignee", "--remove-assignee"} {
		t.Run(flag, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			err := runCmd(t, readyFactory(ios), "12", "--assignee", "john", flag, "jane")
			require.EqualError(t, err, "specify only one of --assignee or --add-assignee/--remove-assignee")
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestEditRun_ChangeAssignees(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		add       []string
		remove    []string
		wantPatch string
	}{
		{name: "remove one", current: `[{"login":"john"},{"login":"jane"}]`, remove: []string{"john"}, wantPatch: `{"assignees":["jane"]}`},
		{name: "remove last sends empty list", current: `[{"login":"john"}]`, remove: []string{"john"}, wantPatch: `{"assignees":[]}`},
		{name: "remove absent is a no-op", current: `[{"login":"jane"}]`, remove: []string{"bob"}, wantPatch: `{"assignees":["jane"]}`},
		{name: "remove from none", current: `null`, remove: []string{"bob"}, wantPatch: `{"assignees":[]}`},
		{name: "add skips present", current: `[{"login":"jane"}]`, add: []string{"jane", "john"}, wantPatch: `{"assignees":["jane","john"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{"number":12,"assignees":`+tt.current+`}`))
			var patched string
			reg.Register(httpmock.REST("PATCH", issuePath), recordBody(&patched, httpmock.StringResponse(http.StatusOK, `{}`)))

			opts, stdout := editOpts(reg)
			opts.AddAssignees = tt.add
			opts.RemoveAssignees = tt.remove
			require.NoError(t, EditRun(opts))
			assert.JSONEq(t, tt.wantPatch, patched)
			assert.Equal(t, "Updated issue #12\n", stdout.String())
		})
	}
}

func TestEditRun_ChangeAssigneesReadErrors(t *testing.T) {
	tests := []struct {
		name    string
		stub    httpmock.Responder
		wantErr string
	}{
		{name: "rejected", stub: httpmock.StringResponse(http.StatusNotFound, `{}`), wantErr: "failed to read issue #12 (HTTP 404)"},
		{name: "transport error", stub: failing, wantErr: "boom"},
		{name: "invalid JSON", stub: httpmock.StringResponse(http.StatusOK, `{`), wantErr: "unexpected end of JSON input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), tt.stub)

			opts, stdout := editOpts(reg)
			opts.RemoveAssignees = []string{"john"}
			err := EditRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Len(t, reg.Requests, 1, "the issue is not patched")
			assert.Empty(t, stdout.String())
		})
	}
}

func TestEditRun_NothingToEditNamesNewFlags(t *testing.T) {
	opts, _ := editOpts(&httpmock.Registry{})
	err := EditRun(opts)
	require.Error(t, err)
	for _, flag := range []string{"--add-assignee", "--remove-assignee"} {
		assert.Contains(t, err.Error(), flag)
	}
}
