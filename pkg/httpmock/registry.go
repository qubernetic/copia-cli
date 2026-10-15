// Package httpmock provides an HTTP round-tripper mock for unit testing
// CLI commands without making real API calls.
package httpmock

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Matcher checks whether an HTTP request matches a stub.
type Matcher func(req *http.Request) bool

// Responder produces a response for a matched request.
type Responder func(req *http.Request) (*http.Response, error)

type stub struct {
	matcher Matcher
	respond Responder
	called  bool
}

// queryTokenParams are the query parameters Copia no longer accepts for auth.
var queryTokenParams = []string{"token", "access_token"}

// Registry is an http.RoundTripper that returns stubbed responses.
type Registry struct {
	mu    sync.Mutex
	stubs []*stub

	// Requests holds every request that reached the registry, in order.
	Requests []*http.Request
}

// Register adds a matcher/responder pair.
func (r *Registry) Register(m Matcher, resp Responder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stubs = append(r.stubs, &stub{matcher: m, respond: resp})
}

// RoundTrip implements http.RoundTripper.
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) {
	// Copia only accepts the Authorization header since 2026-05-01.
	query := req.URL.Query()
	for _, param := range queryTokenParams {
		if query.Has(param) {
			return nil, fmt.Errorf("httpmock: %s %s sends a token as query parameter %q; use the Authorization header", req.Method, req.URL.Path, param)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.Requests = append(r.Requests, req)
	for _, s := range r.stubs {
		if s.matcher(req) {
			s.called = true
			return s.respond(req)
		}
	}
	return nil, fmt.Errorf("no mock matched for %s %s", req.Method, req.URL.Path)
}

// Verify asserts all registered stubs were called.
func (r *Registry) Verify(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range r.stubs {
		if !s.called {
			t.Errorf("httpmock: registered stub was not called")
		}
	}
}

// REST returns a matcher for a REST API call by method and path suffix.
func REST(method, pathSuffix string) Matcher {
	return func(req *http.Request) bool {
		return req.Method == method && strings.HasSuffix(req.URL.Path, pathSuffix)
	}
}

// QueryMatcher returns a matcher like REST that also requires every listed
// query parameter to have exactly the given values. Parameters not listed are
// ignored.
func QueryMatcher(method, pathSuffix string, query url.Values) Matcher {
	rest := REST(method, pathSuffix)
	return func(req *http.Request) bool {
		if !rest(req) {
			return false
		}
		actual := req.URL.Query()
		for key, want := range query {
			if !slices.Equal(actual[key], want) {
				return false
			}
		}
		return true
	}
}

// WithHeader wraps a responder and sets a header on its response.
func WithHeader(responder Responder, name, value string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := responder(req)
		if err != nil {
			return nil, err
		}
		if resp.Header == nil {
			resp.Header = http.Header{}
		}
		resp.Header.Set(name, value)
		return resp, nil
	}
}

// StringResponse returns a responder that sends a fixed status and body.
func StringResponse(status int, body string) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
}
