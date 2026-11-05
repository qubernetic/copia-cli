package edit

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	prPath  = "/api/v1/repos/my-org/my-repo/pulls/7"
	prURL   = "https://app.copia.io/my-org/my-repo/pulls/7"
	updated = `{"number":7,"html_url":"` + prURL + `"}`
)

func runOpts(reg *httpmock.Registry) (*EditOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
	}, stdout
}

func strPtr(s string) *string { return &s }

// recordPatch registers a PATCH stub on path and returns a pointer to the
// body it received.
func recordPatch(reg *httpmock.Registry, path string, status int, resp string) *string {
	var got string
	reg.Register(httpmock.REST("PATCH", path), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		got = string(b)
		return httpmock.StringResponse(status, resp)(req)
	})
	return &got
}

func TestEditRun_Fields(t *testing.T) {
	tests := []struct {
		name  string
		set   func(*EditOptions)
		wantJ string
	}{
		{name: "title", set: func(o *EditOptions) { o.Title = "New title" }, wantJ: `{"title":"New title"}`},
		{name: "body", set: func(o *EditOptions) { o.Body = strPtr("New body") }, wantJ: `{"body":"New body"}`},
		{name: "empty body clears it", set: func(o *EditOptions) { o.Body = strPtr("") }, wantJ: `{"body":""}`},
		{name: "base", set: func(o *EditOptions) { o.Base = "develop" }, wantJ: `{"base":"develop"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			patched := recordPatch(reg, prPath, http.StatusCreated, updated)

			opts, stdout := runOpts(reg)
			tt.set(opts)
			require.NoError(t, EditRun(opts))
			assert.JSONEq(t, tt.wantJ, *patched)
			assert.Equal(t, prURL+"\n", stdout.String())
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
			assert.Equal(t, "application/json", reg.Requests[0].Header.Get("Content-Type"))
		})
	}
}

func TestEditRun_NothingToEdit(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _ := runOpts(reg)
	err := EditRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to edit")
	assert.Empty(t, reg.Requests)
}

const current = `{"number":7,"labels":[{"id":1,"name":"bug"},{"id":2,"name":"plc"}],"assignees":[{"login":"john"}]}`

func stubLabels(reg *httpmock.Registry) {
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"},{"id":2,"name":"plc"},{"id":3,"name":"urgent"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"),
		httpmock.StringResponse(http.StatusOK, `[{"id":9,"name":"org-wide"}]`))
}

func TestEditRun_LabelsAndAssignees(t *testing.T) {
	tests := []struct {
		name   string
		set    func(*EditOptions)
		labels bool
		wantJ  string
	}{
		{
			name:   "add labels",
			set:    func(o *EditOptions) { o.AddLabels = []string{"urgent", "org-wide", "bug"} },
			labels: true,
			wantJ:  `{"labels":[1,2,3,9]}`,
		},
		{
			name:   "remove labels",
			set:    func(o *EditOptions) { o.RemoveLabels = []string{"bug", "urgent"} },
			labels: true,
			wantJ:  `{"labels":[2]}`,
		},
		{
			name:   "remove the last labels sends an empty list",
			set:    func(o *EditOptions) { o.RemoveLabels = []string{"bug", "plc"} },
			labels: true,
			wantJ:  `{"labels":[]}`,
		},
		{
			name:  "add assignees",
			set:   func(o *EditOptions) { o.AddAssignees = []string{"jane", "john"} },
			wantJ: `{"assignees":["john","jane"]}`,
		},
		{
			name:  "remove the last assignee sends an empty list",
			set:   func(o *EditOptions) { o.RemoveAssignees = []string{"john", "nobody"} },
			wantJ: `{"assignees":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.labels {
				stubLabels(reg)
			}
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, current))
			patched := recordPatch(reg, prPath, http.StatusCreated, updated)

			opts, stdout := runOpts(reg)
			tt.set(opts)
			require.NoError(t, EditRun(opts))
			assert.JSONEq(t, tt.wantJ, *patched)
			assert.Equal(t, prURL+"\n", stdout.String())
		})
	}
}

func TestEditRun_AssigneeMe(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"me"}`))
	reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, current))
	patched := recordPatch(reg, prPath, http.StatusCreated, updated)

	opts, _ := runOpts(reg)
	opts.AddAssignees = []string{"@me"}
	opts.RemoveAssignees = []string{"john"}
	require.NoError(t, EditRun(opts))
	assert.JSONEq(t, `{"assignees":["me"]}`, *patched)
}

func TestEditRun_UnknownLabel(t *testing.T) {
	reg := &httpmock.Registry{}
	stubLabels(reg)
	opts, _ := runOpts(reg)
	opts.AddLabels = []string{"nope"}
	require.EqualError(t, EditRun(opts), `label "nope" not found in repository or organization`)
	for _, r := range reg.Requests {
		assert.Equal(t, "GET", r.Method, "no change for an unknown label")
	}
}

func TestEditRun_Milestone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/v1.0"),
		httpmock.StringResponse(http.StatusOK, `{"id":5,"title":"v1.0"}`))
	patched := recordPatch(reg, prPath, http.StatusCreated, updated)

	opts, stdout := runOpts(reg)
	opts.Milestone = "v1.0"
	require.NoError(t, EditRun(opts))
	assert.JSONEq(t, `{"milestone":5}`, *patched)
	assert.Equal(t, prURL+"\n", stdout.String())
}

func TestEditRun_UnknownMilestone(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/nope"),
		httpmock.StringResponse(http.StatusNotFound, `{}`))
	opts, _ := runOpts(reg)
	opts.Milestone = "nope"
	opts.Title = "x"
	require.EqualError(t, EditRun(opts), `milestone "nope" not found in my-org/my-repo`)
	assert.Len(t, reg.Requests, 1, "no change for an unknown milestone")
}

func TestEditRun_RemoveMilestone(t *testing.T) {
	// The pull request PATCH ignores milestone 0, so the issue PATCH of the
	// same index clears it.
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	patched := recordPatch(reg, "/api/v1/repos/my-org/my-repo/issues/7", http.StatusCreated, `{"number":7}`)

	opts, stdout := runOpts(reg)
	opts.RemoveMilestone = true
	require.NoError(t, EditRun(opts))
	assert.JSONEq(t, `{"milestone":0}`, *patched)
	assert.Equal(t, "Updated pull request my-org/my-repo#7\n", stdout.String())
}

func TestEditRun_Reviewers(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	var added, removed string
	reg.Register(httpmock.REST("POST", prPath+"/requested_reviewers"), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		added = string(b)
		return httpmock.StringResponse(http.StatusCreated, `[]`)(req)
	})
	reg.Register(httpmock.REST("DELETE", prPath+"/requested_reviewers"), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		removed = string(b)
		return httpmock.StringResponse(http.StatusNoContent, ``)(req)
	})
	patched := recordPatch(reg, prPath, http.StatusCreated, updated)

	opts, stdout := runOpts(reg)
	opts.Title = "New title"
	opts.AddReviewers = []string{"jane", "my-org/controls"}
	opts.RemoveReviewers = []string{"john"}
	require.NoError(t, EditRun(opts))
	assert.JSONEq(t, `{"title":"New title"}`, *patched)
	assert.JSONEq(t, `{"reviewers":["jane"],"team_reviewers":["controls"]}`, added)
	assert.JSONEq(t, `{"reviewers":["john"]}`, removed)
	assert.Equal(t, prURL+"\n", stdout.String())
}

func TestEditRun_OnlyReviewers(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("DELETE", prPath+"/requested_reviewers"), httpmock.StringResponse(http.StatusNoContent, ``))

	opts, stdout := runOpts(reg)
	opts.RemoveReviewers = []string{"my-org/controls"}
	require.NoError(t, EditRun(opts))
	assert.Len(t, reg.Requests, 1)
	assert.Equal(t, "Updated pull request my-org/my-repo#7\n", stdout.String())
}

func TestEditRun_Conflicts(t *testing.T) {
	tests := []struct {
		name    string
		set     func(*EditOptions)
		wantErr string
	}{
		{
			name:    "milestone and remove-milestone",
			set:     func(o *EditOptions) { o.Milestone = "v1.0"; o.RemoveMilestone = true },
			wantErr: "specify only one of --milestone or --remove-milestone",
		},
		{
			name:    "same label added and removed",
			set:     func(o *EditOptions) { o.AddLabels = []string{"bug"}; o.RemoveLabels = []string{"plc", "bug"} },
			wantErr: `label "bug" cannot be both added and removed`,
		},
		{
			name:    "same assignee added and removed",
			set:     func(o *EditOptions) { o.AddAssignees = []string{"jane"}; o.RemoveAssignees = []string{"jane"} },
			wantErr: `assignee "jane" cannot be both added and removed`,
		},
		{
			name:    "same reviewer added and removed",
			set:     func(o *EditOptions) { o.AddReviewers = []string{"jane"}; o.RemoveReviewers = []string{"jane"} },
			wantErr: `reviewer "jane" cannot be both added and removed`,
		},
		{
			name:    "blank title",
			set:     func(o *EditOptions) { o.Title = " "; o.TitleSet = true },
			wantErr: "title cannot be blank",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			opts, _ := runOpts(reg)
			tt.set(opts)
			require.EqualError(t, EditRun(opts), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestEditRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		set     func(*EditOptions)
		stub    func(*httpmock.Registry)
		wantErr string
	}{
		{
			name: "update refused with message",
			set:  func(o *EditOptions) { o.Base = "nope" },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("PATCH", prPath), httpmock.StringResponse(http.StatusNotFound, `{"message":"new base 'nope' not exist"}`))
			},
			wantErr: "failed to update pull request my-org/my-repo#7: HTTP 404: new base 'nope' not exist",
		},
		{
			name: "update refused",
			set:  func(o *EditOptions) { o.Title = "x" },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("PATCH", prPath), httpmock.StringResponse(http.StatusInternalServerError, `{`))
			},
			wantErr: "failed to update pull request my-org/my-repo#7: HTTP 500",
		},
		{
			name: "read refused",
			set:  func(o *EditOptions) { o.AddAssignees = []string{"jane"} },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "failed to read pull request my-org/my-repo#7: HTTP 404",
		},
		{
			name: "read unreadable",
			set:  func(o *EditOptions) { o.AddAssignees = []string{"jane"} },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, `{`))
			},
			wantErr: "failed to read pull request my-org/my-repo#7",
		},
		{
			name: "label lookup fails",
			set:  func(o *EditOptions) { o.AddLabels = []string{"bug"} },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("GET", "/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusInternalServerError, `{}`))
			},
			wantErr: "500",
		},
		{
			name: "@me lookup fails",
			set:  func(o *EditOptions) { o.AddAssignees = []string{"@me"} },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusUnauthorized, `{}`))
			},
			wantErr: "failed to resolve @me (HTTP 401)",
		},
		{
			name: "remove milestone refused",
			set:  func(o *EditOptions) { o.RemoveMilestone = true },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("PATCH", "/issues/7"), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to remove the milestone of pull request my-org/my-repo#7: HTTP 403",
		},
		{
			name: "add reviewer refused",
			set:  func(o *EditOptions) { o.AddReviewers = []string{"ghost"} },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("POST", "/requested_reviewers"), httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"user does not exist"}`))
			},
			wantErr: "failed to request reviews on pull request my-org/my-repo#7: HTTP 422: user does not exist",
		},
		{
			name: "remove reviewer refused",
			set:  func(o *EditOptions) { o.RemoveReviewers = []string{"ghost"} },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("DELETE", "/requested_reviewers"), httpmock.StringResponse(http.StatusInternalServerError, `{}`))
			},
			wantErr: "failed to remove review requests on pull request my-org/my-repo#7: HTTP 500",
		},
		{
			name: "transport error",
			set:  func(o *EditOptions) { o.Title = "x" },
			stub: func(r *httpmock.Registry) {
				r.Register(httpmock.REST("PATCH", prPath), func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") })
			},
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.stub(reg)
			opts, stdout := runOpts(reg)
			tt.set(opts)
			err := EditRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestEditRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _ := runOpts(reg)
	opts.Host = "bad\x7fhost"
	opts.Title = "x"
	require.Error(t, EditRun(opts))
	assert.Empty(t, reg.Requests)
}
