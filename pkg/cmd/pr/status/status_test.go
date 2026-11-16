package status

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pullsPath  = "/api/v1/repos/my-org/my-repo/pulls"
	searchPath = "/api/v1/repos/issues/search"
)

type pr struct {
	number    int
	title     string
	head      string
	author    string
	updated   string
	state     string // open by default
	merged    bool
	mergeable bool
}

func (p pr) json() string {
	state := p.state
	if state == "" {
		state = "open"
	}
	updated := p.updated
	if updated == "" {
		updated = "2026-11-01T12:00:00Z"
	}
	return fmt.Sprintf(`{"number":%d,"title":%q,"state":%q,"merged":%t,"mergeable":%t,"draft":false,`+
		`"updated_at":%q,"html_url":"https://app.copia.io/my-org/my-repo/pulls/%d","user":{"login":%q},`+
		`"head":{"label":%q,"ref":%q},"base":{"label":"main","ref":"main","repo":{"default_branch":"main"}}}`,
		p.number, p.title, state, p.merged, p.mergeable, updated, p.number, p.author, p.head, p.head)
}

func pulls(prs ...pr) string {
	items := make([]string, 0, len(prs))
	for _, p := range prs {
		items = append(items, p.json())
	}
	return "[" + strings.Join(items, ",") + "]"
}

// hitJSON is one result of the issue search, in the issue format.
func hitJSON(number int, repo string) string {
	return fmt.Sprintf(`{"number":%d,"repository":{"full_name":%q},"pull_request":{"merged":false}}`, number, repo)
}

func hits(items ...string) string {
	return "[" + strings.Join(items, ",") + "]"
}

func registerUser(reg *httpmock.Registry) {
	reg.Register(httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"jane"}`))
}

func registerOpen(reg *httpmock.Registry, body string) {
	reg.Register(httpmock.QueryMatcher("GET", pullsPath, url.Values{
		"state": {"open"}, "sort": {"recentupdate"}, "page": {"1"},
	}), httpmock.StringResponse(http.StatusOK, body))
}

func registerClosed(reg *httpmock.Registry, body string) {
	reg.Register(httpmock.QueryMatcher("GET", pullsPath, url.Values{
		"state": {"closed"}, "sort": {"recentupdate"}, "page": {"1"},
	}), httpmock.StringResponse(http.StatusOK, body))
}

func registerReview(reg *httpmock.Registry, body string) {
	reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{
		"type": {"pulls"}, "state": {"open"}, "review_requested": {"true"}, "owner": {"my-org"}, "page": {"1"},
	}), httpmock.StringResponse(http.StatusOK, body))
}

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams, branch string) *StatusOptions {
	return &StatusOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Branch: func() (string, error) {
			if branch == "" {
				return "", errors.New("not on any branch")
			}
			return branch, nil
		},
	}
}

func TestStatusRun(t *testing.T) {
	sensor := pr{number: 7, title: "Add sensor mapping", head: "feature/sensor", author: "jane", updated: "2026-11-05T09:00:00Z"}
	older := pr{number: 5, title: "Older change", head: "feature/old", author: "jane", updated: "2026-11-02T09:00:00Z"}
	others := pr{number: 9, title: "Review me", head: "feature/review", author: "bob", updated: "2026-11-04T09:00:00Z"}
	notMine := pr{number: 3, title: "Not for me", head: "feature/other", author: "bob", updated: "2026-11-03T09:00:00Z"}

	var many []pr
	for n := 30; n < 42; n++ {
		many = append(many, pr{number: n, title: fmt.Sprintf("Change %d", n), head: fmt.Sprintf("b%d", n), author: "jane"})
	}

	tests := []struct {
		name         string
		branch       string
		repoOverride bool
		conflict     bool
		open         string
		closed       string // registered when not empty
		review       string
		want         string
	}{
		{
			name:   "three sections",
			branch: "feature/sensor",
			// Out of order, the newest update comes first.
			open:   pulls(older, notMine, sensor, others),
			review: hits(hitJSON(9, "my-org/my-repo"), hitJSON(9, "my-org/other-repo"), hitJSON(3, "my-org/other-repo")),
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  #7  Add sensor mapping [feature/sensor]

Created by you
  #7  Add sensor mapping [feature/sensor]
  #5  Older change [feature/old]

Requesting a code review from you
  #9  Review me [feature/review]

`,
		},
		{
			name:   "branch without a pull request",
			branch: "feature/none",
			open:   pulls(others),
			closed: pulls(pr{number: 2, title: "Other branch", head: "feature/x", author: "jane", state: "closed"}),
			review: "[]",
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  There is no pull request associated with [feature/none]

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:   "merged pull request of the branch",
			branch: "feature/done",
			open:   "[]",
			closed: pulls(pr{number: 4, title: "Done", head: "feature/done", author: "jane", state: "closed", merged: true}),
			review: "[]",
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  #4  Done [feature/done] - Merged

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:   "closed pull request of the default branch hidden",
			branch: "main",
			open:   "[]",
			closed: pulls(pr{number: 4, title: "From main", head: "main", author: "jane", state: "closed"}),
			review: "[]",
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  There is no pull request associated with [main]

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:   "no current branch",
			open:   "[]",
			review: "[]",
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  There is no current branch

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:         "repository override drops the current branch",
			branch:       "feature/sensor",
			repoOverride: true,
			open:         pulls(sensor),
			review:       "[]",
			want: `
Relevant pull requests in my-org/my-repo

Created by you
  #7  Add sensor mapping [feature/sensor]

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:     "conflict status",
			branch:   "feature/sensor",
			conflict: true,
			open:     pulls(pr{number: 7, title: "Add sensor mapping", head: "feature/sensor", author: "jane", mergeable: true}, pr{number: 5, title: "Older change", head: "feature/old", author: "jane"}),
			review:   "[]",
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  #7  Add sensor mapping [feature/sensor] ✓ No merge conflicts

Created by you
  #7  Add sensor mapping [feature/sensor] ✓ No merge conflicts
  #5  Older change [feature/old] × Merge conflicts

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:         "more than ten",
			repoOverride: true,
			open:         pulls(many...),
			review:       "[]",
			want: `
Relevant pull requests in my-org/my-repo

Created by you
  #30  Change 30 [b30]
  #31  Change 31 [b31]
  #32  Change 32 [b32]
  #33  Change 33 [b33]
  #34  Change 34 [b34]
  #35  Change 35 [b35]
  #36  Change 36 [b36]
  #37  Change 37 [b37]
  #38  Change 38 [b38]
  #39  Change 39 [b39]
  And 2 more

Requesting a code review from you
  You have no pull requests to review

`,
		},
		{
			name:   "long title truncated",
			branch: "b1",
			open:   pulls(pr{number: 1, title: strings.Repeat("x", 60), head: "b1", author: "bob"}),
			review: "[]",
			want: `
Relevant pull requests in my-org/my-repo

Current branch
  #1  ` + strings.Repeat("x", 47) + `... [b1]

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			registerUser(reg)
			registerOpen(reg, tt.open)
			if tt.closed != "" {
				registerClosed(reg, tt.closed)
			}
			registerReview(reg, tt.review)

			ios, _, stdout, _ := iostreams.Test()
			opts := newOpts(reg, ios, tt.branch)
			opts.HasRepoOverride = tt.repoOverride
			opts.ConflictStatus = tt.conflict

			require.NoError(t, StatusRun(opts))
			assert.Equal(t, tt.want, stdout.String())
		})
	}
}

func TestStatusRun_ClosedOnePage(t *testing.T) {
	// Only the most recently updated page of closed pull requests is read:
	// a repository can have thousands, and the list has no head filter.
	var closed []pr
	for n := 1; n <= cmdutil.MaxPageSize; n++ {
		closed = append(closed, pr{number: n, title: "Old", head: fmt.Sprintf("old-%d", n), author: "bob", state: "closed"})
	}

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	registerUser(reg)
	registerOpen(reg, "[]")
	reg.Register(httpmock.QueryMatcher("GET", pullsPath, url.Values{
		"state": {"closed"}, "sort": {"recentupdate"}, "page": {"1"}, "limit": {fmt.Sprint(cmdutil.MaxPageSize)},
	}), httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, pulls(closed...)),
		"Link", `<https://app.copia.io/api/v1/repos/my-org/my-repo/pulls?page=2>; rel="next"`))
	registerReview(reg, "[]")

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, StatusRun(newOpts(reg, ios, "feature/none")))
	assert.Contains(t, stdout.String(), "There is no pull request associated with [feature/none]")
}

func TestStatusRun_JSON(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		open   string
		closed string
		review string
		want   string
	}{
		{
			name:   "three sections",
			branch: "feature/sensor",
			open: pulls(
				pr{number: 7, title: "Add sensor mapping", head: "feature/sensor", author: "jane", updated: "2026-11-05T09:00:00Z"},
				pr{number: 9, title: "Review me", head: "feature/review", author: "bob"},
			),
			review: hits(hitJSON(9, "my-org/my-repo")),
			want: `{
				"currentBranch": {"number": 7, "state": "open", "head": {"label": "feature/sensor", "ref": "feature/sensor"}},
				"createdBy": [{"number": 7, "state": "open", "head": {"label": "feature/sensor", "ref": "feature/sensor"}}],
				"needsReview": [{"number": 9, "state": "open", "head": {"label": "feature/review", "ref": "feature/review"}}]
			}`,
		},
		{
			name:   "empty sections",
			branch: "feature/done",
			open:   "[]",
			closed: pulls(pr{number: 4, title: "Done", head: "feature/done", author: "jane", state: "closed", merged: true}),
			review: "[]",
			want: `{
				"currentBranch": {"number": 4, "state": "merged", "head": {"label": "feature/done", "ref": "feature/done"}},
				"createdBy": [],
				"needsReview": []
			}`,
		},
		{
			name:   "branch without a pull request",
			branch: "feature/none",
			open:   "[]",
			closed: "[]",
			review: "[]",
			want:   `{"currentBranch": null, "createdBy": [], "needsReview": []}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			registerUser(reg)
			registerOpen(reg, tt.open)
			if tt.closed != "" {
				registerClosed(reg, tt.closed)
			}
			registerReview(reg, tt.review)

			ios, _, stdout, _ := iostreams.Test()
			opts := newOpts(reg, ios, tt.branch)
			opts.JSON = cmdutil.JSONFlags{Fields: []string{"number", "state", "head"}}

			require.NoError(t, StatusRun(opts))
			assert.JSONEq(t, tt.want, stdout.String())
		})
	}
}

func TestStatusRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		user    httpmock.Responder
		open    httpmock.Responder
		closed  httpmock.Responder
		review  httpmock.Responder
		wantErr string
	}{
		{
			name:    "current user",
			user:    httpmock.StringResponse(http.StatusUnauthorized, `{}`),
			wantErr: "HTTP 401",
		},
		{
			name:    "open pull requests",
			open:    httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "API error (HTTP 404)",
		},
		{
			name:    "closed pull requests",
			branch:  "feature/none",
			open:    httpmock.StringResponse(http.StatusOK, `[]`),
			closed:  httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "API error (HTTP 500)",
		},
		{
			name:    "review requests",
			open:    httpmock.StringResponse(http.StatusOK, `[]`),
			review:  httpmock.StringResponse(http.StatusBadRequest, `{}`),
			wantErr: "API error (HTTP 400)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			user := tt.user
			if user == nil {
				user = httpmock.StringResponse(http.StatusOK, `{"login":"jane"}`)
			}
			reg.Register(httpmock.REST("GET", "/api/v1/user"), user)
			if tt.open != nil {
				reg.Register(httpmock.QueryMatcher("GET", pullsPath, url.Values{"state": {"open"}}), tt.open)
			}
			if tt.closed != nil {
				reg.Register(httpmock.QueryMatcher("GET", pullsPath, url.Values{"state": {"closed"}}), tt.closed)
			}
			if tt.review != nil {
				reg.Register(httpmock.REST("GET", searchPath), tt.review)
			}

			ios, _, stdout, _ := iostreams.Test()
			err := StatusRun(newOpts(reg, ios, tt.branch))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdStatus(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdStatus(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	registerUser(reg)
	registerOpen(reg, pulls(pr{number: 7, title: "Mine", head: "feature/sensor", author: "jane"}))
	registerReview(reg, "[]")

	ios, _, stdout, _ := iostreams.Test()
	// -R skips the current branch, so no git call is made.
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runCmd(f, "-c", "--json", "number,mergeable", "--jq", ".createdBy[].number, .currentBranch == null"))
	assert.Equal(t, "7\ntrue\n", stdout.String())
}

func TestNewCmdStatus_Errors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	err := runCmd(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}, "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")

	err = runCmd(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "bad"})
	require.Error(t, err)
}
