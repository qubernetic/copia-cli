package cmdutil

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// MaxPageSize is the largest page FetchPages asks the API for.
const MaxPageSize = 50

// HTTPError is returned by FetchPages when the API answers with a status
// other than 200.
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("API error (HTTP %d)", e.StatusCode)
}

// PageDecoder decodes the body of one page into its items.
type PageDecoder[T any] func(body []byte) ([]T, error)

// FetchPages GETs rawURL page by page and returns up to limit items. A limit
// of 0 fetches every page. The page and limit query parameters are set on
// rawURL; other parameters are kept on every request.
func FetchPages[T any](client *http.Client, rawURL, token string, limit int, decode PageDecoder[T]) ([]T, error) {
	return fetchPages(client, rawURL, token, limit, decode, nil)
}

// FetchFilteredPages is FetchPages for a filter the API cannot apply: it
// drops every decoded item keep rejects and counts limit on the kept items.
// It asks for full pages, since the share of kept items is not known, and a
// page whose items are all dropped does not end the listing.
func FetchFilteredPages[T any](client *http.Client, rawURL, token string, limit int, decode PageDecoder[T], keep func(T) bool) ([]T, error) {
	return fetchPages(client, rawURL, token, limit, decode, keep)
}

// fetchPages implements FetchPages and, with a non-nil keep, FetchFilteredPages.
func fetchPages[T any](client *http.Client, rawURL, token string, limit int, decode PageDecoder[T], keep func(T) bool) ([]T, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	all := []T{}
	for page := 1; ; page++ {
		pageSize := MaxPageSize
		if keep == nil && limit > 0 && limit-len(all) < pageSize {
			pageSize = limit - len(all)
		}

		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		q.Set("limit", strconv.Itoa(pageSize))
		u.RawQuery = q.Encode()

		body, header, err := getPage(client, u.String(), token)
		if err != nil {
			return nil, err
		}
		items, err := decode(body)
		if err != nil {
			return nil, fmt.Errorf("parsing response: %w", err)
		}
		if len(items) == 0 {
			return all, nil
		}
		for _, it := range items {
			if keep == nil || keep(it) {
				all = append(all, it)
			}
		}

		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		if !hasNextPage(header, len(items), pageSize) {
			return all, nil
		}
	}
}

// hasNextPage reports whether another page follows. A Link header decides
// when present; without one, a page shorter than requested is the last.
func hasNextPage(header http.Header, got, requested int) bool {
	if link := header.Get("Link"); link != "" {
		return strings.Contains(link, `rel="next"`)
	}
	return got >= requested
}

// linkRE matches one entry of a Link header, as gh's api command does.
var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

// FollowLinkPages sends req and then, for as long as a response's Link
// header names a rel="next" page, a copy of req for that page without a
// body. page gets every response in order and must not close its body;
// an error from page ends the walk. A next link to another scheme or host
// is refused, so the Authorization header of req never leaves its host, and
// a next link to a page already fetched is an error, so a looping server
// cannot keep the walk going forever.
func FollowLinkPages(client *http.Client, req *http.Request, page func(*http.Response) error) error {
	origin := req.URL
	fetched := map[string]bool{}
	for {
		fetched[req.URL.String()] = true
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", req.URL.Host, err)
		}
		err = page(resp)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}

		next, err := nextPageURL(req.URL, resp.Header)
		if err != nil || next == nil {
			return err
		}
		if next.Scheme != origin.Scheme || next.Host != origin.Host {
			return fmt.Errorf("refusing to follow the next page link to another host: %q", next.String())
		}
		if fetched[next.String()] {
			return fmt.Errorf("the next page link repeats an already fetched page: %q", next.String())
		}
		req = req.Clone(req.Context())
		req.URL = next
		req.Host = ""
		req.Body = nil
		req.GetBody = nil
		req.ContentLength = 0
	}
}

// nextPageURL returns the rel="next" target of a Link header, resolved
// against base, or nil when there is none.
func nextPageURL(base *url.URL, header http.Header) (*url.URL, error) {
	for _, m := range linkRE.FindAllStringSubmatch(header.Get("Link"), -1) {
		if !slices.Contains(strings.Fields(m[2]), "next") {
			continue
		}
		u, err := url.Parse(m[1])
		if err != nil {
			return nil, fmt.Errorf("parsing the next page link: %w", err)
		}
		return base.ResolveReference(u), nil
	}
	return nil, nil
}

// getPage sends one authenticated GET and returns the body and headers of a
// 200 response.
func getPage(client *http.Client, pageURL, token string) ([]byte, http.Header, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to %s: %w", req.URL.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, &HTTPError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header, nil
}
