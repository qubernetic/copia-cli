package shared

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const releaseJSON = `{
	"id": 42,
	"tag_name": "v1.0.0",
	"name": "First",
	"body": "notes",
	"draft": false,
	"prerelease": true,
	"created_at": "2026-01-02T03:04:05Z",
	"published_at": "2026-01-03T03:04:05Z",
	"author": {"id": 7, "login": "alice"},
	"html_url": "https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0",
	"tarball_url": "https://app.copia.io/my-org/my-repo/archive/v1.0.0.tar.gz",
	"zipball_url": "https://app.copia.io/my-org/my-repo/archive/v1.0.0.zip",
	"target_commitish": "main",
	"assets": [{"id": 5, "name": "a.zip", "size": 2048, "download_count": 3, "browser_download_url": "https://app.copia.io/attachments/x", "uuid": "x"}]
}`

func client(reg *httpmock.Registry) *http.Client {
	return &http.Client{Transport: reg}
}

func TestFetchRelease_ByTag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"),
		httpmock.StringResponse(http.StatusOK, releaseJSON),
	)

	rel, err := FetchRelease(client(reg), "app.copia.io", "test-token", "my-org", "my-repo", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, &Release{
		ID:              42,
		TagName:         "v1.0.0",
		Name:            "First",
		Body:            "notes",
		Prerelease:      true,
		CreatedAt:       "2026-01-02T03:04:05Z",
		PublishedAt:     "2026-01-03T03:04:05Z",
		Author:          Author{Login: "alice"},
		HTMLURL:         "https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0",
		TarballURL:      "https://app.copia.io/my-org/my-repo/archive/v1.0.0.tar.gz",
		ZipballURL:      "https://app.copia.io/my-org/my-repo/archive/v1.0.0.zip",
		TargetCommitish: "main",
		Assets: []Asset{{
			ID: 5, Name: "a.zip", Size: 2048, DownloadCount: 3,
			BrowserDownloadURL: "https://app.copia.io/attachments/x",
		}},
	}, rel)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestFetchRelease_Latest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/latest"),
		httpmock.StringResponse(http.StatusOK, releaseJSON),
	)

	rel, err := FetchRelease(client(reg), "app.copia.io", "test-token", "my-org", "my-repo", "")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", rel.TagName)
}

func TestFetchRelease_EscapesTag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/release/1.0#rc"),
		httpmock.StringResponse(http.StatusOK, releaseJSON),
	)

	_, err := FetchRelease(client(reg), "app.copia.io", "test-token", "my-org", "my-repo", "release/1.0#rc")
	require.NoError(t, err)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "/api/v1/repos/my-org/my-repo/releases/tags/release%2F1.0%23rc", reg.Requests[0].URL.EscapedPath())
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestFetchRelease_Errors(t *testing.T) {
	const tagPath = "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"
	const latestPath = "/api/v1/repos/my-org/my-repo/releases/latest"

	tests := []struct {
		name    string
		host    string
		tag     string
		path    string
		resp    httpmock.Responder
		wantErr string
	}{
		{
			name:    "invalid host",
			host:    "bad\x7fhost",
			tag:     "v1.0.0",
			wantErr: "invalid control character",
		},
		{
			name: "transport error",
			tag:  "v1.0.0",
			path: tagPath,
			resp: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:    "tag not found",
			tag:     "v1.0.0",
			path:    tagPath,
			resp:    httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "release v1.0.0 not found",
		},
		{
			name:    "no latest release",
			path:    latestPath,
			resp:    httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "release not found",
		},
		{
			name:    "server error",
			tag:     "v1.0.0",
			path:    tagPath,
			resp:    httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "API error (HTTP 500)",
		},
		{
			name: "body read error",
			tag:  "v1.0.0",
			path: tagPath,
			resp: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{}), Header: http.Header{}}, nil
			},
			wantErr: "read failed",
		},
		{
			name:    "malformed JSON",
			tag:     "v1.0.0",
			path:    tagPath,
			resp:    httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.resp != nil {
				reg.Register(httpmock.REST("GET", tt.path), tt.resp)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			rel, err := FetchRelease(client(reg), host, "test-token", "my-org", "my-repo", tt.tag)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, rel)
		})
	}
}
