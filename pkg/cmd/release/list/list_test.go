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
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"tag_name":"v1.0.0","name":"Release 1.0.0","draft":false,"prerelease":false,"published_at":"2026-03-30T10:00:00Z"},
			{"id":2,"tag_name":"v0.9.0","name":"Beta release","draft":false,"prerelease":true,"published_at":"2026-03-29T10:00:00Z"}
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
	assert.Contains(t, stdout.String(), "v1.0.0")
	assert.Contains(t, stdout.String(), "v0.9.0")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"tag_name":"v1.0.0","name":"Release 1.0.0","draft":false,"prerelease":false,"published_at":"2026-03-30T10:00:00Z"}
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
		JSON:       cmdutil.JSONFlags{Fields: []string{"tagName", "name"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"tag_name"`)
}

// releasesJSON renders count releases numbered from first.
func releasesJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"id":%d,"tag_name":"v0.0.%d","name":"Release %d","draft":false,"prerelease":false,"published_at":"2026-03-30T10:00:00Z"}`, first+i, first+i, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/my-repo/releases"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, releasesJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, releasesJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, releasesJSON(101, 20)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      120,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "v0.0.120")
	require.Len(t, reg.Requests, 3)
}

func TestListRun_StatusColumn(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusOK, `[
			{"id":1,"tag_name":"v1.0.0","name":"Stable","draft":false,"prerelease":false},
			{"id":2,"tag_name":"v1.1.0-rc.1","name":"RC","draft":false,"prerelease":true},
			{"id":3,"tag_name":"v2.0.0","name":"Next","draft":true,"prerelease":true}
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

	require.NoError(t, ListRun(opts))
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.Len(t, lines, 3)
	assert.Regexp(t, `^v1\.0\.0\s+Stable\s+release$`, lines[0])
	assert.Regexp(t, `^v1\.1\.0-rc\.1\s+RC\s+pre-release$`, lines[1])
	assert.Regexp(t, `^v2\.0\.0\s+Next\s+draft$`, lines[2])
}

func TestListRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "invalid limit",
			limit:   0,
			wantErr: "invalid limit: 0",
		},
		{
			name:    "server error",
			limit:   30,
			respond: httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "API error (HTTP 500)",
		},
		{
			name:    "malformed JSON",
			limit:   30,
			respond: httpmock.StringResponse(http.StatusOK, `{not json`),
			wantErr: "parsing response",
		},
		{
			name:  "transport error",
			limit: 30,
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
			if tt.respond != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"), tt.respond)
			}

			ios, _, stdout, _ := iostreams.Test()
			opts := &ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Limit:      tt.limit,
			}

			err := ListRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdList_Flags(t *testing.T) {
	cmd := NewCmdList(&cmdutil.Factory{})
	assert.Equal(t, []string{"ls"}, cmd.Aliases)

	flag := cmd.Flags().ShorthandLookup("L")
	require.NotNil(t, flag)
	assert.Equal(t, "limit", flag.Name)
	assert.Equal(t, "30", flag.DefValue)
	assert.NotNil(t, cmd.Flags().Lookup("json"))
}

func TestNewCmdList_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/my-repo/releases", pageQuery(1, 5)),
		httpmock.StringResponse(http.StatusOK, releasesJSON(1, 2)),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"--limit", "5", "--json", "tag_name"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, stdout.String(), `"tag_name": "v0.0.1"`)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdList_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdList(tt.factory)
			cmd.SetArgs([]string{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}
