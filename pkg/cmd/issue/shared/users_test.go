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

func TestResolveAssignees(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		logins    []string
		responder httpmock.Responder
		want      []string
		wantErr   string
	}{
		{name: "no @me sends no request", logins: []string{"john", "jane"}, want: []string{"john", "jane"}},
		{
			name:      "@me resolves once",
			logins:    []string{"@me", "jane", "@me"},
			responder: httpmock.StringResponse(http.StatusOK, `{"login":"dennis"}`),
			want:      []string{"dennis", "jane", "dennis"},
		},
		{
			name:      "API error",
			logins:    []string{"@me"},
			responder: httpmock.StringResponse(http.StatusUnauthorized, ``),
			wantErr:   "failed to resolve @me (HTTP 401)",
		},
		{
			name:      "transport error",
			logins:    []string{"@me"},
			responder: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr:   "connecting to app.copia.io: ",
		},
		{
			name:   "body read error",
			logins: []string{"@me"},
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}, nil
			},
			wantErr: "read failed",
		},
		{name: "malformed json", logins: []string{"@me"}, responder: httpmock.StringResponse(http.StatusOK, `{not json`), wantErr: "invalid character"},
		{name: "bad url", host: "bad host", logins: []string{"@me"}, wantErr: "invalid character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/user"), tt.responder)
			}

			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			got, err := ResolveAssignees(&http.Client{Transport: reg}, host, "test-token", tt.logins)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			if tt.responder != nil {
				assert.Len(t, reg.Requests, 1)
				assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
			} else {
				assert.Empty(t, reg.Requests)
			}
		})
	}
}
