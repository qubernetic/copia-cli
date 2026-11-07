package cmdutil

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// maxRedirects matches the limit of Go's default redirect policy.
const maxRedirects = 10

// NewHTTPClient returns the HTTP client every command uses. It follows
// redirects only within the origin (scheme, host and port) of the first
// request, so the Authorization header never reaches another server or
// travels over plain HTTP. A cross-origin redirect fails the request instead
// of being followed without the header.
//
// Transport stays nil, so the client uses http.DefaultTransport and
// httpmock.ReplaceDefaultTransport keeps working in tests.
func NewHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: checkSameOriginRedirect}
}

func checkSameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	from, to := via[0].URL, req.URL
	if !sameOrigin(from, to) {
		return errors.New("refusing redirect from " + from.Scheme + "://" + from.Host +
			" to " + to.Scheme + "://" + to.Host + ": it changes the scheme, host or port")
	}
	return nil
}

// maxDownloadRedirects bounds the redirects of a download; Copia sends an
// asset download to its storage with one.
const maxDownloadRedirects = 3

// NewDownloadHTTPClient returns the client for the request that downloads a
// release asset or archive, the one exception to the same-origin rule:
// Copia answers it with a redirect to presigned storage on another host.
// It follows https redirects only, at most maxDownloadRedirects, and once a
// hop leaves the origin of the first request no request carries the
// Authorization header or another credential. Every other request uses
// NewHTTPClient.
func NewDownloadHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: checkDownloadRedirect}
}

func checkDownloadRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxDownloadRedirects {
		return fmt.Errorf("stopped after %d redirects", maxDownloadRedirects)
	}
	from, to := via[0].URL, req.URL
	if to.Scheme != "https" {
		return errors.New("refusing redirect from " + from.Scheme + "://" + from.Host +
			" to " + to.Scheme + "://" + to.Host + ": a download follows https redirects only")
	}
	left := !sameOrigin(from, to)
	for _, r := range via[1:] {
		left = left || !sameOrigin(from, r.URL)
	}
	if left {
		req.Header.Del("Authorization")
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Cookie")
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Host, b.Host)
}
