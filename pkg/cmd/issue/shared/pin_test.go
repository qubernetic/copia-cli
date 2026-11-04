package shared

import (
	"errors"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pinIssuePath = "/api/v1/repos/my-org/my-repo/issues/12"

func TestFetchPinState(t *testing.T) {
	tests := []struct {
		name       string
		respond    httpmock.Responder
		wantTitle  string
		wantPinned bool
		wantKnown  bool
		wantErr    string
	}{
		{
			name:       "pinned",
			respond:    httpmock.StringResponse(http.StatusOK, `{"title":"Broken","pin_order":2}`),
			wantTitle:  "Broken",
			wantPinned: true,
			wantKnown:  true,
		},
		{
			name:      "not pinned",
			respond:   httpmock.StringResponse(http.StatusOK, `{"title":"Broken","pin_order":0}`),
			wantTitle: "Broken",
			wantKnown: true,
		},
		{
			name:      "server without pin_order",
			respond:   httpmock.StringResponse(http.StatusOK, `{"title":"Broken"}`),
			wantTitle: "Broken",
		},
		{
			name:    "not found",
			respond: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "issue my-org/my-repo#12 not found",
		},
		{
			name:    "other status",
			respond: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "failed to read issue my-org/my-repo#12 (HTTP 500)",
		},
		{
			name:    "invalid JSON",
			respond: httpmock.StringResponse(http.StatusOK, `{`),
			wantErr: "unexpected end of JSON input",
		},
		{
			name:    "transport error",
			respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", pinIssuePath), tt.respond)

			state, err := FetchPinState(&http.Client{Transport: reg}, "app.copia.io", "test-token", "my-org", "my-repo", 12)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTitle, state.Title)
			pinned, known := state.Pinned()
			assert.Equal(t, tt.wantPinned, pinned)
			assert.Equal(t, tt.wantKnown, known)
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
		})
	}
}

func TestSetPinned(t *testing.T) {
	tests := []struct {
		name    string
		pin     bool
		respond httpmock.Responder
		wantErr string
	}{
		{name: "pin", pin: true, respond: httpmock.StringResponse(http.StatusNoContent, "")},
		{name: "unpin", pin: false, respond: httpmock.StringResponse(http.StatusNoContent, "")},
		{
			name:    "pin forbidden",
			pin:     true,
			respond: httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: "cannot pin issue my-org/my-repo#12: permission denied (HTTP 403); pinning an issue needs write access to the repository's issues",
		},
		{
			name:    "unpin forbidden",
			pin:     false,
			respond: httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: "cannot unpin issue my-org/my-repo#12: permission denied (HTTP 403); unpinning an issue needs write access to the repository's issues",
		},
		{
			name:    "not found",
			pin:     true,
			respond: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "issue my-org/my-repo#12 not found",
		},
		{
			name:    "server message",
			pin:     true,
			respond: httpmock.StringResponse(http.StatusBadRequest, `{"message":"maximum number of pinned issues reached"}`),
			wantErr: "failed to pin issue my-org/my-repo#12 (HTTP 400): maximum number of pinned issues reached",
		},
		{
			name:    "other status",
			pin:     false,
			respond: httpmock.StringResponse(http.StatusInternalServerError, ``),
			wantErr: "failed to unpin issue my-org/my-repo#12 (HTTP 500)",
		},
		{
			name:    "transport error",
			pin:     true,
			respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := "DELETE"
			if tt.pin {
				method = "POST"
			}
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST(method, pinIssuePath+"/pin"), tt.respond)

			err := SetPinned(&http.Client{Transport: reg}, "app.copia.io", "test-token", "my-org", "my-repo", 12, tt.pin)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
		})
	}
}

func TestPinRequests_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	client := &http.Client{Transport: reg}
	_, err := FetchPinState(client, "bad\x7fhost", "t", "o", "r", 1)
	require.Error(t, err)
	require.Error(t, SetPinned(client, "bad\x7fhost", "t", "o", "r", 1, true))
	assert.Empty(t, reg.Requests)
}
