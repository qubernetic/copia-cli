package cmdutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pageURL = "https://app.copia.io/api/v1/repos/o/r/issues?state=open"

type item struct {
	N int `json:"n"`
}

func decodeItems(body []byte) ([]item, error) {
	var items []item
	err := json.Unmarshal(body, &items)
	return items, err
}

// itemsJSON renders count items numbered from first.
func itemsJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"n":%d}`, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func requestedPages(reg *httpmock.Registry) []string {
	var got []string
	for _, req := range reg.Requests {
		q := req.URL.Query()
		got = append(got, q.Get("page")+"/"+q.Get("limit"))
	}
	return got
}

func TestFetchPages_SplitsLimitIntoPages(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, itemsJSON(101, 20)))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 120, decodeItems)
	require.NoError(t, err)
	require.Len(t, items, 120)
	assert.Equal(t, 1, items[0].N)
	assert.Equal(t, 120, items[119].N)
	assert.Equal(t, []string{"1/50", "2/50", "3/20"}, requestedPages(reg))
}

func TestFetchPages_StopsOnEmptyPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, `[]`))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 120, decodeItems)
	require.NoError(t, err)
	assert.Len(t, items, 50)
	assert.Equal(t, []string{"1/50", "2/50"}, requestedPages(reg))
}

func TestFetchPages_StopsOnLinkWithoutNext(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)), "Link",
			`<https://app.copia.io/api/v1/repos/o/r/issues?page=2&limit=50>; rel="next",<https://app.copia.io/api/v1/repos/o/r/issues?page=2&limit=50>; rel="last"`),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, itemsJSON(51, 50)), "Link",
			`<https://app.copia.io/api/v1/repos/o/r/issues?page=1&limit=50>; rel="first",<https://app.copia.io/api/v1/repos/o/r/issues?page=1&limit=50>; rel="prev"`),
	)

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 120, decodeItems)
	require.NoError(t, err)
	assert.Len(t, items, 100)
	assert.Equal(t, []string{"1/50", "2/50"}, requestedPages(reg))
}

func TestFetchPages_StopsOnShortPageWithoutLink(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 22)))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 120, decodeItems)
	require.NoError(t, err)
	assert.Len(t, items, 22)
	assert.Equal(t, []string{"1/50"}, requestedPages(reg))
}

func TestFetchPages_FollowsNextPastShortPage(t *testing.T) {
	// A server whose page cap is below 50 returns short pages but still
	// announces the next one.
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, itemsJSON(1, 30)), "Link",
			`<https://app.copia.io/api/v1/repos/o/r/issues?page=2&limit=30>; rel="next"`),
	)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 20)), httpmock.StringResponse(http.StatusOK, itemsJSON(31, 20)))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 50, decodeItems)
	require.NoError(t, err)
	assert.Len(t, items, 50)
	assert.Equal(t, []string{"1/50", "2/20"}, requestedPages(reg))
}

func TestFetchPages_SmallLimitSendsOneRequest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 10)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 10)))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 10, decodeItems)
	require.NoError(t, err)
	assert.Len(t, items, 10)
	assert.Equal(t, []string{"1/10"}, requestedPages(reg))
}

func TestFetchPages_ZeroLimitFetchesAllPages(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(3, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(101, 7)))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 0, decodeItems)
	require.NoError(t, err)
	assert.Len(t, items, 107)
	assert.Equal(t, []string{"1/50", "2/50", "3/50"}, requestedPages(reg))
}

func TestFetchPages_KeepsQueryAndSendsTokenInHeader(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 10)), httpmock.StringResponse(http.StatusOK, itemsJSON(51, 10)))

	_, err := FetchPages(&http.Client{Transport: reg}, pageURL+"&labels=bug", "test-token", 60, decodeItems)
	require.NoError(t, err)
	require.Len(t, reg.Requests, 2)
	for _, req := range reg.Requests {
		q := req.URL.Query()
		assert.Equal(t, "open", q.Get("state"))
		assert.Equal(t, "bug", q.Get("labels"))
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.NotContains(t, req.URL.String(), "test-token")
	}
}

func TestFetchPages_HTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)), httpmock.StringResponse(http.StatusNotFound, `{}`))

	_, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 120, decodeItems)
	require.Error(t, err)
	assert.Equal(t, "API error (HTTP 404)", err.Error())

	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestFetchPages_DecodeError(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(httpmock.REST("GET", "/issues"), httpmock.StringResponse(http.StatusOK, `not json`))

	_, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 30, decodeItems)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing response")
}

func TestFetchPages_NoItemsReturnsEmptySlice(t *testing.T) {
	// Callers print the result as JSON; an empty list must stay [] and not
	// become null.
	reg := &httpmock.Registry{}
	reg.Register(httpmock.REST("GET", "/issues"), httpmock.StringResponse(http.StatusOK, `[]`))

	items, err := FetchPages(&http.Client{Transport: reg}, pageURL, "test-token", 30, decodeItems)
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}
