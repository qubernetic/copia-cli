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

func TestListRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"tag_name":"v1.0.0","name":"Release 1.0.0","draft":false,"prerelease":false,"published_at":"2026-03-30T10:00:00Z"},
			{"id":2,"tag_name":"v0.9.0","name":"Beta release","draft":false,"prerelease":true,"published_at":"2026-03-29T10:00:00Z"}
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
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "v1.0.0")
	assert.Contains(t, stdout.String(), "v0.9.0")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"tag_name":"v1.0.0","name":"Release 1.0.0","draft":false,"prerelease":false,"published_at":"2026-03-30T10:00:00Z"}
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
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"tagName", "name"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"tag_name"`)
}

// releasesJSON renders count releases numbered from first.
func releasesJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"id":%d,"tag_name":"v0.0.%d","name":"Release %d","draft":false,"prerelease":false,"published_at":"2026-03-30T10:00:00Z"}`, first+i, first+i, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/my-repo/releases"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, releasesJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, releasesJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, releasesJSON(101, 20)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      120,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "v0.0.120")
	require.Len(t, reg.Requests, 3)
}
