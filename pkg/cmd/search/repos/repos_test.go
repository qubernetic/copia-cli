package repos

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

func TestSearchRepos_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/search"),
		httpmock.StringResponse(http.StatusOK, `{"data":[
			{"full_name":"my-org/plc-project","description":"PLC code","html_url":"https://app.copia.io/my-org/plc-project"},
			{"full_name":"other/hmi","description":"HMI config","html_url":"https://app.copia.io/other/hmi"}
		]}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Limit:      30,
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "plc-project")
}

func TestSearchRepos_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/search"),
		httpmock.StringResponse(http.StatusOK, `{"data":[
			{"full_name":"my-org/plc","description":"PLC","html_url":""}
		]}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"fullName"}},
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"full_name"`)
}

// searchJSON renders a search response wrapping count repositories numbered
// from first.
func searchJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"full_name":"my-org/plc-%d","description":"PLC %d","html_url":"https://app.copia.io/my-org/plc-%d"}`, first+i, first+i, first+i)
	}
	return `{"ok":true,"data":[` + strings.Join(parts, ",") + "]}"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestSearchRepos_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/search"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, searchJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, searchJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, searchJSON(101, 20)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Limit:      120,
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "PLC 1\n")
	assert.Contains(t, stdout.String(), "PLC 51\n")
	assert.Contains(t, stdout.String(), "PLC 120\n")

	require.Len(t, reg.Requests, 3)
	for _, req := range reg.Requests {
		assert.Equal(t, "plc", req.URL.Query().Get("q"))
	}
}
