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
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":7,"title":"feat: add cylinder wrapper","state":"open","user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feature/cylinder"},"updated_at":"2026-03-30T10:00:00Z"},
			{"number":6,"title":"fix: sensor timeout","state":"open","user":{"login":"jane"},"base":{"label":"main"},"head":{"label":"fix/sensor"},"updated_at":"2026-03-29T10:00:00Z"}
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
	assert.Contains(t, stdout.String(), "add cylinder wrapper")
	assert.Contains(t, stdout.String(), "sensor timeout")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":7,"title":"feat: add cylinder wrapper","state":"open","user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feature/cylinder"},"updated_at":"2026-03-30T10:00:00Z"}
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

// prsJSON renders count pull requests numbered from first.
func prsJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"number":%d,"title":"PR %d","state":"open","user":{"login":"john"},"base":{"label":"main"},"head":{"label":"feature/x"},"updated_at":"2026-03-30T10:00:00Z"}`, first+i, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/my-repo/pulls"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, prsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, prsJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, prsJSON(101, 20)))

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
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "#120")

	require.Len(t, reg.Requests, 3)
	for _, req := range reg.Requests {
		assert.Equal(t, "closed", req.URL.Query().Get("state"))
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
	}
}
