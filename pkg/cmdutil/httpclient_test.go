package cmdutil

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func redirectRequest(t *testing.T, raw string) *http.Request {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return &http.Request{URL: u, Header: http.Header{}}
}

func TestCheckSameOriginRedirect(t *testing.T) {
	origin := "https://app.copia.io/api/v1/repos"
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"same origin", "https://app.copia.io/api/v1/other", false},
		{"scheme only", "http://app.copia.io/api/v1/repos", true},
		{"scheme only with explicit port", "http://app.copia.io:443/api/v1/repos", true},
		{"other port", "https://app.copia.io:8443/api/v1/repos", true},
		{"other host", "https://evil.example/api/v1/repos", true},
		{"subdomain", "https://api.app.copia.io/api/v1/repos", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			via := []*http.Request{redirectRequest(t, origin)}
			err := checkSameOriginRedirect(redirectRequest(t, tt.target), via)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			target, _ := url.Parse(tt.target)
			assert.Contains(t, err.Error(), "https://app.copia.io")
			assert.Contains(t, err.Error(), target.Scheme+"://"+target.Host)
		})
	}
}

func TestCheckSameOriginRedirect_ComparesWithFirstRequest(t *testing.T) {
	via := []*http.Request{
		redirectRequest(t, "https://app.copia.io/a"),
		redirectRequest(t, "https://app.copia.io/b"),
	}
	assert.NoError(t, checkSameOriginRedirect(redirectRequest(t, "https://app.copia.io/c"), via))
}

func TestCheckSameOriginRedirect_StopsAfterTenRedirects(t *testing.T) {
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = redirectRequest(t, "https://app.copia.io/a")
	}
	err := checkSameOriginRedirect(redirectRequest(t, "https://app.copia.io/b"), via)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10 redirects")
}

func TestNewHTTPClient_KeepsDefaultTransport(t *testing.T) {
	assert.Nil(t, NewHTTPClient().Transport)
}

// authCounter records how many requests reached a handler and how many of
// them carried an Authorization header.
type authCounter struct {
	requests, withAuth atomic.Int32
}

func (c *authCounter) record(r *http.Request) {
	c.requests.Add(1)
	if r.Header.Get("Authorization") != "" {
		c.withAuth.Add(1)
	}
}

func TestNewHTTPClient_RefusesCrossOriginRedirect(t *testing.T) {
	var target authCounter
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target.record(r)
	}))
	defer plain.Close()
	otherPort := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target.record(r)
	}))
	defer otherPort.Close()

	tests := []struct {
		name     string
		location string
	}{
		// plain listens on the same host (127.0.0.1) as the TLS origin.
		{"downgrade to http", plain.URL + "/leak"},
		{"other port", otherPort.URL + "/leak"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, tt.location, http.StatusFound)
			}))
			defer origin.Close()
			require.True(t, strings.HasPrefix(tt.location, "http"))

			client := NewHTTPClient()
			client.Transport = origin.Client().Transport
			req, err := http.NewRequest(http.MethodGet, origin.URL+"/api/v1/user", nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "token secret")

			resp, err := client.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "refusing redirect")
		})
	}
	assert.Zero(t, target.requests.Load(), "redirect target was contacted")
	assert.Zero(t, target.withAuth.Load(), "redirect target received the token")
}

func TestNewHTTPClient_FollowsSameOriginRedirect(t *testing.T) {
	var got authCounter
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, srv.URL+"/new", http.StatusFound)
			return
		}
		got.record(r)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewHTTPClient()
	client.Transport = srv.Client().Transport
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/old", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "token secret")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), got.requests.Load())
	assert.Equal(t, int32(1), got.withAuth.Load())
}
