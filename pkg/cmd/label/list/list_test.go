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
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"name":"bug","color":"#e11d48","description":"Something isn't working"},
			{"id":2,"name":"feature","color":"#0969da","description":"New feature request"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "bug")
	assert.Contains(t, stdout.String(), "feature")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"name":"bug","color":"#e11d48","description":"Something isn't working"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"name", "color"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"name"`)
	assert.Contains(t, stdout.String(), "bug")
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

func labelsJSON(first, count int) string {
	items := make([]string, count)
	for i := range items {
		n := first + i
		items[i] = fmt.Sprintf(`{"id":%d,"name":"label-%d","color":"#000000","description":""}`, n, n)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/my-repo/labels"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, labelsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 30)), httpmock.StringResponse(http.StatusOK, labelsJSON(51, 30)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      80,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 80, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "label-80")
	assert.Len(t, reg.Requests, 2)
}

func TestListRun_DefaultLimitCapsOutput(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/my-repo/labels", pageQuery(1, 30)),
		httpmock.StringResponse(http.StatusOK, labelsJSON(1, 30)),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 30, strings.Count(stdout.String(), "\n"))
	assert.Len(t, reg.Requests, 1)
}

func TestListRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "not found",
			respond: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "API error (HTTP 404)",
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
			reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), tt.respond)

			ios, _, stdout, _ := iostreams.Test()
			opts := &ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Limit:      30,
			}

			err := ListRun(opts)
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
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/my-repo/labels", pageQuery(1, 2)),
		httpmock.StringResponse(http.StatusOK, labelsJSON(1, 2)),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"-L", "2"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, 2, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "label-2")
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdList_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			wantErr: "could not determine repository",
		},
		{
			name:    "invalid limit",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--limit", "-1"},
			wantErr: "invalid limit: -1",
		},
		{
			name:    "non-numeric limit",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--limit", "many"},
			wantErr: "invalid argument \"many\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdList(tt.factory)
			cmd.SetArgs(tt.args)
			if tt.args == nil {
				cmd.SetArgs([]string{})
			}
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}
