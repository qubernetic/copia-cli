package cmdutil

import (
	"errors"
	"fmt"
	"net/http"
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
	if from.Scheme != to.Scheme || !strings.EqualFold(from.Host, to.Host) {
		return errors.New("refusing redirect from " + from.Scheme + "://" + from.Host +
			" to " + to.Scheme + "://" + to.Host + ": it changes the scheme, host or port")
	}
	return nil
}
