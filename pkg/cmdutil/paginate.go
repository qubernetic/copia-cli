package cmdutil

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MaxPageSize is the largest page FetchPages asks the API for.
const MaxPageSize = 50

// PageDecoder decodes the body of one page into its items.
type PageDecoder[T any] func(body []byte) ([]T, error)

// FetchPages GETs rawURL page by page and returns up to limit items. A limit
// of 0 fetches every page. The page and limit query parameters are set on
// rawURL; other parameters are kept on every request.
func FetchPages[T any](client *http.Client, rawURL, token string, limit int, decode PageDecoder[T]) ([]T, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	all := []T{}
	for page := 1; ; page++ {
		pageSize := MaxPageSize
		if limit > 0 && limit-len(all) < pageSize {
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
		all = append(all, items...)

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
		return nil, nil, fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header, nil
}
