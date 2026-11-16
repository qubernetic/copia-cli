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

const milestonesPath = "/api/v1/repos/my-org/my-repo/milestones/"

func TestResolveMilestoneID(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		milestone string
		path      string
		responder httpmock.Responder
		want      int64
		wantRaw   string
		wantErr   string
	}{
		{
			name:      "by name",
			milestone: "v1.0",
			path:      milestonesPath + "v1.0",
			responder: httpmock.StringResponse(http.StatusOK, `{"id":4,"title":"v1.0"}`),
			want:      4,
		},
		{
			name:      "by numeric ID",
			milestone: "1",
			path:      milestonesPath + "1",
			responder: httpmock.StringResponse(http.StatusOK, `{"id":1,"title":"Sprint 1"}`),
			want:      1,
		},
		{
			name:      "name is path escaped",
			milestone: "release/Q4 2026",
			path:      milestonesPath + "release/Q4 2026",
			responder: httpmock.StringResponse(http.StatusOK, `{"id":7,"title":"release/Q4 2026"}`),
			want:      7,
			wantRaw:   milestonesPath + "release%2FQ4%202026",
		},
		{
			name:      "not found names the milestone",
			milestone: "v9",
			path:      milestonesPath + "v9",
			responder: httpmock.StringResponse(http.StatusNotFound, `{"message":"not found"}`),
			wantErr:   `milestone "v9" not found in my-org/my-repo`,
		},
		{
			name:      "other API error",
			milestone: "v1.0",
			path:      milestonesPath + "v1.0",
			responder: httpmock.StringResponse(http.StatusInternalServerError, `oops`),
			wantErr:   `failed to look up milestone "v1.0" (HTTP 500)`,
		},
		{
			name:      "transport error",
			milestone: "v1.0",
			path:      milestonesPath + "v1.0",
			responder: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr:   "connecting to app.copia.io: ",
		},
		{
			name:      "body read error",
			milestone: "v1.0",
			path:      milestonesPath + "v1.0",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}, nil
			},
			wantErr: "read failed",
		},
		{
			name:      "malformed json",
			milestone: "v1.0",
			path:      milestonesPath + "v1.0",
			responder: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr:   "invalid character",
		},
		{name: "bad url", host: "bad host", milestone: "v1.0", wantErr: "invalid character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", tt.path), tt.responder)
			}

			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			id, err := ResolveMilestoneID(&http.Client{Transport: reg}, host, "test-token", "my-org", "my-repo", tt.milestone)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, id)
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
			if tt.wantRaw != "" {
				assert.Equal(t, tt.wantRaw, reg.Requests[0].URL.EscapedPath())
			}
		})
	}
}
