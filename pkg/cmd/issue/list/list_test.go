package list

import (
	"fmt"
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

func TestListRun_InvalidState(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &ListOptions{
		IO:    ios,
		State: "invalid",
		Limit: 30,
	}

	err := ListRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid state")
}

func TestListRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/issues"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":12,"title":"Fix PLC connection timeout","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[{"name":"bug"}]},
			{"number":11,"title":"Add safety interlock","state":"open","updated_at":"2026-03-29T10:00:00Z","labels":[]}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "open",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Fix PLC connection timeout")
	assert.Contains(t, stdout.String(), "12")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/issues"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":12,"title":"Fix PLC","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[]}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "open",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"number", "title"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"number"`)
}

func TestListRun_LabelFilter(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/issues"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":12,"title":"Fix PLC timeout","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[{"name":"bug"}]}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "open",
		Limit:      30,
		Labels:     []string{"bug"},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Fix PLC timeout")
}

// issuesJSON renders count issues numbered from first.
func issuesJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"number":%d,"title":"Issue %d","state":"open","updated_at":"2026-03-30T10:00:00Z","labels":[]}`, first+i, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/my-repo/issues"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, issuesJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, issuesJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, issuesJSON(101, 20)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "closed",
		Limit:      120,
		Labels:     []string{"bug", "plc"},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "#120")

	require.Len(t, reg.Requests, 3)
	for _, req := range reg.Requests {
		q := req.URL.Query()
		assert.Equal(t, "closed", q.Get("state"))
		assert.Equal(t, "bug,plc", q.Get("labels"))
		assert.Equal(t, "issues", q.Get("type"))
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.NotContains(t, req.URL.String(), "test-token")
	}
}

func TestListRun_SmallLimitSendsOneRequest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/my-repo/issues", pageQuery(1, 10)),
		httpmock.StringResponse(http.StatusOK, issuesJSON(1, 10)),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "open",
		Limit:      10,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 10, strings.Count(stdout.String(), "\n"))
	assert.Len(t, reg.Requests, 1)
}

func TestListRun_FiltersOnEveryPage(t *testing.T) {
	tests := []struct {
		name  string
		set   func(*ListOptions)
		param string
		want  string
	}{
		{"assignee", func(o *ListOptions) { o.Assignee = "jane" }, "assigned_by", "jane"},
		{"author", func(o *ListOptions) { o.Author = "john" }, "created_by", "john"},
		{"mention", func(o *ListOptions) { o.Mention = "joe" }, "mentioned_by", "joe"},
		{"milestone", func(o *ListOptions) { o.Milestone = "v1.0 beta" }, "milestones", "v1.0 beta"},
		{"search", func(o *ListOptions) { o.Search = "plc timeout&x" }, "q", "plc timeout&x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			path := "/api/v1/repos/my-org/my-repo/issues"
			for page, limit := range map[int]int{1: 50, 2: 10} {
				q := pageQuery(page, limit)
				q.Set(tt.param, tt.want)
				reg.Register(httpmock.QueryMatcher("GET", path, q), httpmock.StringResponse(http.StatusOK, issuesJSON(1+(page-1)*50, limit)))
			}

			ios, _, stdout, _ := iostreams.Test()
			opts := &ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				State:      "open",
				Limit:      60,
			}
			tt.set(opts)

			require.NoError(t, ListRun(opts))
			assert.Equal(t, 60, strings.Count(stdout.String(), "\n"))
			require.Len(t, reg.Requests, 2)
		})
	}
}

func TestListRun_MeResolvedOnce(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"jane"}`))
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/my-repo/issues", url.Values{"assigned_by": {"jane"}, "created_by": {"jane"}}),
		httpmock.StringResponse(http.StatusOK, `[]`),
	)

	ios, _, _, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "open",
		Limit:      30,
		Assignee:   "@me",
		Author:     "@me",
	}

	require.NoError(t, ListRun(opts))
	require.Len(t, reg.Requests, 2)
	assert.Equal(t, "/api/v1/user", reg.Requests[0].URL.Path)
}

func TestListRun_MeResolveError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusUnauthorized, `{}`))

	ios, _, _, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		State:      "open",
		Limit:      30,
		Author:     "@me",
	}

	err := ListRun(opts)
	require.EqualError(t, err, "failed to resolve @me (HTTP 401)")
}
