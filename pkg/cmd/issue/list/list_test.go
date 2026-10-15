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
