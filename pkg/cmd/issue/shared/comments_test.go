package shared

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const commentsPath = "/api/v1/repos/my-org/my-repo/issues/7/comments"

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestFetchComments(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", commentsPath), httpmock.StringResponse(http.StatusOK,
		`[{"body":"Looks good","user":{"login":"jane"},"created_at":"2026-03-30T10:00:00Z"}]`))

	comments, err := FetchComments(&http.Client{Transport: reg}, "app.copia.io", "test-token", "my-org", "my-repo", 7)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "jane", comments[0].User.Login)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestFetchComments_Errors(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		responder httpmock.Responder
		wantErr   string
	}{
		{name: "bad url", host: "bad host", wantErr: "invalid character"},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{name: "not found", responder: httpmock.StringResponse(http.StatusNotFound, ``), wantErr: "API error (HTTP 404)"},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}, nil
			},
			wantErr: "read failed",
		},
		{name: "malformed json", responder: httpmock.StringResponse(http.StatusOK, `{not json`), wantErr: "invalid character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", commentsPath), tt.responder)
			}

			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			_, err := FetchComments(&http.Client{Transport: reg}, host, "test-token", "my-org", "my-repo", 7)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPrintComments(t *testing.T) {
	var buf bytes.Buffer
	var c Comment
	c.User.Login = "jane"
	c.CreatedAt = "2026-03-30T10:00:00Z"
	c.Body = "Looks good"

	PrintComments(&buf, []Comment{c})
	assert.Equal(t, "\njane commented 2026-03-30T10:00:00Z:\nLooks good\n", buf.String())

	buf.Reset()
	PrintComments(&buf, nil)
	assert.Empty(t, buf.String())
}
