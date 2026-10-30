package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
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

func isEven(it item) bool { return it.N%2 == 0 }

func isAbove(n int) func(item) bool {
	return func(it item) bool { return it.N > n }
}

func TestFetchFilteredPages_DroppedPageDoesNotEndListing(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, itemsJSON(1, 3)), "Link",
			`<https://app.copia.io/api/v1/repos/o/r/issues?page=2&limit=50>; rel="next"`),
	)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(4, 3)))

	items, err := FetchFilteredPages(&http.Client{Transport: reg}, pageURL, "test-token", 1, decodeItems, isAbove(3))
	require.NoError(t, err)
	assert.Equal(t, []item{{N: 4}}, items)
	assert.Equal(t, []string{"1/50", "2/50"}, requestedPages(reg))
}

func TestFetchFilteredPages_CountsLimitOnKeptItems(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(51, 50)))

	items, err := FetchFilteredPages(&http.Client{Transport: reg}, pageURL, "test-token", 30, decodeItems, isEven)
	require.NoError(t, err)
	require.Len(t, items, 30)
	assert.Equal(t, 2, items[0].N)
	assert.Equal(t, 60, items[29].N)
	assert.Equal(t, []string{"1/50", "2/50"}, requestedPages(reg))
}

func TestFetchFilteredPages_StopsOnShortPageWithoutLink(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, itemsJSON(1, 7)))

	items, err := FetchFilteredPages(&http.Client{Transport: reg}, pageURL, "test-token", 30, decodeItems, isEven)
	require.NoError(t, err)
	assert.Equal(t, []item{{N: 2}, {N: 4}, {N: 6}}, items)
	assert.Equal(t, []string{"1/50"}, requestedPages(reg))
}

const linkedURL = "https://app.copia.io/api/v1/repos/o/r/issues?state=open"

// linkedPage registers page n of a listing whose Link header names next as
// the following page; next 0 ends the listing.
func linkedPage(reg *httpmock.Registry, n, next int, body string) {
	query := url.Values{"page": nil}
	if n > 1 {
		query = url.Values{"page": {fmt.Sprint(n)}}
	}
	responder := httpmock.StringResponse(http.StatusOK, body)
	if next > 0 {
		responder = httpmock.WithHeader(responder, "Link", fmt.Sprintf(
			`<https://app.copia.io/api/v1/repos/o/r/issues?state=open&page=%d>; rel="next",<https://app.copia.io/api/v1/repos/o/r/issues?state=open&page=9>; rel="last"`, next))
	}
	reg.Register(httpmock.QueryMatcher("GET", "/issues", query), responder)
}

func newLinkedRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", linkedURL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "token test-token")
	req.Header.Set("X-Custom", "kept")
	return req
}

// collectBodies is a page callback that keeps every body.
func collectBodies(bodies *[]string) func(*http.Response) error {
	return func(resp *http.Response) error {
		b, err := io.ReadAll(resp.Body)
		*bodies = append(*bodies, string(b))
		return err
	}
}

func TestFollowLinkPages_FollowsNextUntilNone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	linkedPage(reg, 1, 2, `[1]`)
	linkedPage(reg, 2, 3, `[2]`)
	linkedPage(reg, 3, 0, `[3]`)

	var bodies []string
	err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), collectBodies(&bodies))
	require.NoError(t, err)
	assert.Equal(t, []string{`[1]`, `[2]`, `[3]`}, bodies)
	require.Len(t, reg.Requests, 3)
	for _, req := range reg.Requests {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "open", req.URL.Query().Get("state"))
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "kept", req.Header.Get("X-Custom"))
	}
}

func TestFollowLinkPages_SinglePageWithoutLink(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	linkedPage(reg, 1, 0, `{"a":1}`)

	var bodies []string
	err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), collectBodies(&bodies))
	require.NoError(t, err)
	assert.Equal(t, []string{`{"a":1}`}, bodies)
}

func TestFollowLinkPages_ResolvesRelativeLink(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", url.Values{"page": nil}),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, `[1]`), "Link", `</api/v1/repos/o/r/issues?page=9>; rel="last", </api/v1/repos/o/r/issues?page=2>; rel="next"`))
	linkedPage(reg, 2, 0, `[2]`)

	var bodies []string
	err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), collectBodies(&bodies))
	require.NoError(t, err)
	assert.Equal(t, []string{`[1]`, `[2]`}, bodies)
	assert.Equal(t, "app.copia.io", reg.Requests[1].URL.Host)
}

func TestFollowLinkPages_StopsOnRepeatedLink(t *testing.T) {
	tests := []struct {
		name     string
		register func(reg *httpmock.Registry)
		repeated string
		requests int
	}{
		{
			name: "self link",
			register: func(reg *httpmock.Registry) {
				reg.Register(httpmock.QueryMatcher("GET", "/issues", url.Values{"page": nil}),
					httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, `[1]`), "Link", `<`+linkedURL+`>; rel="next"`))
			},
			repeated: linkedURL,
			requests: 1,
		},
		{
			name: "page 2 links back to page 1",
			register: func(reg *httpmock.Registry) {
				linkedPage(reg, 1, 2, `[1]`)
				reg.Register(httpmock.QueryMatcher("GET", "/issues", url.Values{"page": {"2"}}),
					httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, `[2]`), "Link", `<`+linkedURL+`>; rel="next"`))
			},
			repeated: linkedURL,
			requests: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.register(reg)

			var bodies []string
			err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), collectBodies(&bodies))
			require.EqualError(t, err, fmt.Sprintf("the next page link repeats an already fetched page: %q", tt.repeated))
			assert.Len(t, bodies, tt.requests, "every fetched page is still handed over")
			assert.Len(t, reg.Requests, tt.requests)
		})
	}
}

func TestFollowLinkPages_ChecksNextLinkOrigin(t *testing.T) {
	tests := []struct {
		name    string
		link    string
		refused string
	}{
		{name: "other host", link: "https://evil.example/api/v1/repos/o/r/issues?page=2", refused: "https://evil.example/api/v1/repos/o/r/issues?page=2"},
		{name: "plain http on the same host", link: "http://app.copia.io/api/v1/repos/o/r/issues?page=2", refused: "http://app.copia.io/api/v1/repos/o/r/issues?page=2"},
		{name: "same host on another port", link: "https://app.copia.io:8443/api/v1/repos/o/r/issues?page=2", refused: "https://app.copia.io:8443/api/v1/repos/o/r/issues?page=2"},
		{name: "scheme-relative link to another host", link: "//other.example/api/v1/repos/o/r/issues?page=2", refused: "https://other.example/api/v1/repos/o/r/issues?page=2"},
		{name: "upper-case host", link: "https://APP.COPIA.IO/api/v1/repos/o/r/issues?page=2", refused: "https://APP.COPIA.IO/api/v1/repos/o/r/issues?page=2"},
		{name: "trailing dot", link: "https://app.copia.io./api/v1/repos/o/r/issues?page=2", refused: "https://app.copia.io./api/v1/repos/o/r/issues?page=2"},
		{name: "user info on the same host", link: "https://user@app.copia.io/api/v1/repos/o/r/issues?page=2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.QueryMatcher("GET", "/issues", url.Values{"page": nil}),
				httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, `[1]`), "Link", `<`+tt.link+`>; rel="next"`))
			if tt.refused == "" {
				linkedPage(reg, 2, 0, `[2]`)
			}

			var bodies []string
			err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), collectBodies(&bodies))

			if tt.refused != "" {
				require.EqualError(t, err, fmt.Sprintf("refusing to follow the next page link to another host: %q", tt.refused))
				assert.Equal(t, []string{`[1]`}, bodies, "the first page is still handed over")
				assert.Len(t, reg.Requests, 1, "the token is never sent to the other origin")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{`[1]`, `[2]`}, bodies)
			require.Len(t, reg.Requests, 2)
			auth := reg.Requests[1].Header.Get("Authorization")
			assert.Equal(t, "token test-token", auth)
			assert.NotContains(t, auth, "Basic")
		})
	}
}

func TestFollowLinkPages_StopsOnCallbackError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	linkedPage(reg, 1, 2, `[1]`)

	err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), func(*http.Response) error { return assert.AnError })
	require.ErrorIs(t, err, assert.AnError)
	assert.Len(t, reg.Requests, 1)
}

func TestFollowLinkPages_ConnectionError(t *testing.T) {
	reg := &httpmock.Registry{}
	err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), func(*http.Response) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connecting to app.copia.io")
}

func TestFollowLinkPages_InvalidLink(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", "/issues", url.Values{"page": nil}),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, `[1]`), "Link", "<http://[::1>; rel=\"next\""))

	err := FollowLinkPages(&http.Client{Transport: reg}, newLinkedRequest(t), func(*http.Response) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing the next page link")
}
