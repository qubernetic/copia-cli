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

func TestCheckDownloadRedirect(t *testing.T) {
	origin := "https://app.copia.io/o/r/releases/download/v1/a.zip"
	tests := []struct {
		name     string
		target   string
		wantAuth bool
		wantErr  string
	}{
		{name: "same origin keeps the token", target: "https://app.copia.io/attachments/x", wantAuth: true},
		{name: "other host drops the token", target: "https://s3.example/bucket/x?X-Amz-Signature=s"},
		{name: "subdomain drops the token", target: "https://storage.app.copia.io/x"},
		{name: "other port drops the token", target: "https://app.copia.io:8443/x"},
		{name: "http refused", target: "http://s3.example/bucket/x?X-Amz-Signature=s", wantErr: "refusing redirect from https://app.copia.io to http://s3.example: a download follows https redirects only"},
		{name: "http on the same host refused", target: "http://app.copia.io/x", wantErr: "refusing redirect from https://app.copia.io to http://app.copia.io: a download follows https redirects only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := redirectRequest(t, tt.target)
			req.Header.Set("Authorization", "token secret")
			req.Header.Set("Cookie", "session=secret")
			req.Header.Set("Proxy-Authorization", "secret")
			err := checkDownloadRedirect(req, []*http.Request{redirectRequest(t, origin)})
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantAuth {
				assert.Equal(t, "token secret", req.Header.Get("Authorization"))
				return
			}
			assert.Empty(t, req.Header.Get("Authorization"))
			assert.Empty(t, req.Header.Get("Cookie"))
			assert.Empty(t, req.Header.Get("Proxy-Authorization"))
		})
	}
}

func TestCheckDownloadRedirect_NoTokenAfterLeavingTheOrigin(t *testing.T) {
	via := []*http.Request{
		redirectRequest(t, "https://app.copia.io/a"),
		redirectRequest(t, "https://s3.example/b"),
	}
	req := redirectRequest(t, "https://app.copia.io/c")
	req.Header.Set("Authorization", "token secret")
	require.NoError(t, checkDownloadRedirect(req, via))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestCheckDownloadRedirect_StopsAfterThreeRedirects(t *testing.T) {
	via := make([]*http.Request, 3)
	for i := range via {
		via[i] = redirectRequest(t, "https://app.copia.io/a")
	}
	err := checkDownloadRedirect(redirectRequest(t, "https://s3.example/b?X-Amz-Signature=s"), via)
	require.EqualError(t, err, "stopped after 3 redirects")
}

func TestNewDownloadHTTPClient_FollowsCrossOriginRedirectWithoutToken(t *testing.T) {
	var storage authCounter
	store := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storage.record(r)
		_, _ = w.Write([]byte("asset"))
	}))
	defer store.Close()
	// Both servers listen on 127.0.0.1, so only the port differs: Go's own
	// header stripping keeps the token there, the download client does not.
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, store.URL+"/bucket/a.zip?X-Amz-Signature=s", http.StatusFound)
	}))
	defer origin.Close()

	client := NewDownloadHTTPClient()
	assert.Nil(t, client.Transport)
	client.Transport = origin.Client().Transport
	req, err := http.NewRequest(http.MethodGet, origin.URL+"/o/r/releases/download/v1/a.zip", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "token secret")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), storage.requests.Load())
	assert.Zero(t, storage.withAuth.Load(), "storage received the token")
}

func TestNewHTTPClient_StillRefusesWhatDownloadsFollow(t *testing.T) {
	via := []*http.Request{redirectRequest(t, "https://app.copia.io/a")}
	err := checkSameOriginRedirect(redirectRequest(t, "https://s3.example/b"), via)
	require.Error(t, err)
	assert.Nil(t, NewHTTPClient().Transport)
}
