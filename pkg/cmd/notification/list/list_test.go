package list

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/notifications"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"subject":{"title":"Fix PLC timeout","type":"Issue"},"repository":{"full_name":"my-org/plc"},"unread":true},
			{"id":2,"subject":{"title":"Add sensor","type":"Pull"},"repository":{"full_name":"my-org/plc"},"unread":true}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Fix PLC timeout")
	assert.Contains(t, stdout.String(), "Add sensor")
}

func TestListRun_Empty(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/notifications"),
		httpmock.StringResponse(http.StatusOK, `[]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "No unread notifications")
}

func TestListRun_AllFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/notifications"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"subject":{"title":"Old PR","type":"Pull"},"repository":{"full_name":"my-org/plc"},"unread":false}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		All:        true,
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Old PR")
}

func TestNewCmdList_LimitDefault(t *testing.T) {
	cmd := NewCmdList(&cmdutil.Factory{})
	flag := cmd.Flags().ShorthandLookup("L")
	require.NotNil(t, flag)
	assert.Equal(t, "limit", flag.Name)
	assert.Equal(t, "30", flag.DefValue)
}

func TestListRun_InvalidLimit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	err := ListRun(&ListOptions{IO: ios, Limit: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")
}

func notificationsJSON(first, count int) string {
	items := make([]string, count)
	for i := range items {
		n := first + i
		items[i] = fmt.Sprintf(`{"id":%d,"subject":{"title":"Notification %d","type":"Issue"},"repository":{"full_name":"my-org/plc"},"unread":true}`, n, n)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/notifications"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, notificationsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 30)), httpmock.StringResponse(http.StatusOK, notificationsJSON(51, 30)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		All:        true,
		Limit:      80,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 80, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "Notification 80")

	require.Len(t, reg.Requests, 2)
	for _, req := range reg.Requests {
		assert.Equal(t, "true", req.URL.Query().Get("all"))
	}
}

func TestListRun_DefaultLimitCapsOutput(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/notifications", pageQuery(1, 30)),
		httpmock.StringResponse(http.StatusOK, notificationsJSON(1, 30)),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 30, strings.Count(stdout.String(), "\n"))
	require.Len(t, reg.Requests, 1)
	assert.Empty(t, reg.Requests[0].URL.Query().Get("all"))
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/notifications"),
		httpmock.StringResponse(http.StatusOK, `[{"id":7,"subject":{"title":"Fix PLC timeout","type":"Issue"},"repository":{"full_name":"my-org/plc"},"unread":true}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"id"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"id": 7`)
	assert.Contains(t, stdout.String(), `"full_name": "my-org/plc"`)
}

func TestListRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "server error",
			respond: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "HTTP 500",
		},
		{
			name:    "malformed JSON",
			respond: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr: "parsing response",
		},
		{
			name: "transport error",
			respond: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/notifications"), tt.respond)

			ios, _, stdout, _ := iostreams.Test()
			err := ListRun(&ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Limit:      30,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdList_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/notifications", url.Values{"all": {"true"}, "page": {"1"}, "limit": {"5"}}),
		httpmock.StringResponse(http.StatusOK, `[{"id":1,"subject":{"title":"Old PR","type":"Pull"},"repository":{"full_name":"my-org/plc"},"unread":false}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"--all", "-L", "5"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "my-org/plc  Pull  Old PR")
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdList_AuthError(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	cmd := NewCmdList(f)
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}
