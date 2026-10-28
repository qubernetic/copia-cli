package list

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRun_InvalidLimit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &ListOptions{
		IO:    ios,
		Limit: -1,
	}

	err := ListRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")
}

func TestListRun_UserRepos(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user/repos"),
		httpmock.StringResponse(http.StatusOK, `[
			{"full_name":"john/plc-project","description":"PLC code","private":false,"updated_at":"2026-03-30T10:00:00Z"},
			{"full_name":"john/hmi-config","description":"HMI setup","private":true,"updated_at":"2026-03-29T10:00:00Z"}
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
	assert.Contains(t, stdout.String(), "john/plc-project")
	assert.Contains(t, stdout.String(), "john/hmi-config")
}

func TestListRun_OrgRepos(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/orgs/my-org/repos"),
		httpmock.StringResponse(http.StatusOK, `[
			{"full_name":"my-org/main-plc","description":"Main PLC","private":false,"updated_at":"2026-03-30T10:00:00Z"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Org:        "my-org",
		Limit:      30,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "my-org/main-plc")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user/repos"),
		httpmock.StringResponse(http.StatusOK, `[
			{"full_name":"john/plc-project","description":"PLC code","private":false,"updated_at":"2026-03-30T10:00:00Z"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"fullName", "description"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "john/plc-project")
	assert.Contains(t, stdout.String(), "PLC code")
}

// reposJSON renders count repositories numbered from first.
func reposJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"full_name":"my-org/repo-%d","description":"","private":false,"updated_at":"2026-03-30T10:00:00Z"}`, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	tests := []struct {
		name string
		org  string
		path string
	}{
		{name: "user", path: "/api/v1/user/repos"},
		{name: "org", org: "my-org", path: "/api/v1/orgs/my-org/repos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.QueryMatcher("GET", tt.path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, reposJSON(1, 50)))
			reg.Register(httpmock.QueryMatcher("GET", tt.path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, reposJSON(51, 50)))
			reg.Register(httpmock.QueryMatcher("GET", tt.path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, reposJSON(101, 20)))

			ios, _, stdout, _ := iostreams.Test()

			opts := &ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Org:        tt.org,
				Limit:      120,
			}

			err := ListRun(opts)
			require.NoError(t, err)
			assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
			assert.Contains(t, stdout.String(), "my-org/repo-120")
			require.Len(t, reg.Requests, 3)
		})
	}
}

const mixedReposJSON = `[
	{"full_name":"john/plc-project","description":"PLC code","private":false,"updated_at":"2026-03-30T10:00:00Z"},
	{"full_name":"john/hmi-config","description":"HMI setup","private":true,"updated_at":"2026-03-29T10:00:00Z"}
]`

func TestListRun_Visibility(t *testing.T) {
	tests := []struct {
		name       string
		visibility string
		json       bool
		want       []string
		notWant    []string
	}{
		{name: "private table", visibility: "private", want: []string{"john/hmi-config"}, notWant: []string{"john/plc-project"}},
		{name: "public table", visibility: "public", want: []string{"john/plc-project"}, notWant: []string{"john/hmi-config"}},
		{name: "all table", want: []string{"john/plc-project", "john/hmi-config"}},
		{name: "private json", visibility: "private", json: true, want: []string{`"full_name": "john/hmi-config"`}, notWant: []string{"john/plc-project"}},
		{name: "public json", visibility: "public", json: true, want: []string{`"full_name": "john/plc-project"`}, notWant: []string{"john/hmi-config"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/user/repos"), httpmock.StringResponse(http.StatusOK, mixedReposJSON))

			ios, _, stdout, _ := iostreams.Test()
			opts := &ListOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Limit:      30,
				Visibility: tt.visibility,
			}
			if tt.json {
				opts.JSON = cmdutil.JSONFlags{Fields: []string{"full_name"}}
			}

			require.NoError(t, ListRun(opts))
			for _, s := range tt.want {
				assert.Contains(t, stdout.String(), s)
			}
			for _, s := range tt.notWant {
				assert.NotContains(t, stdout.String(), s)
			}
		})
	}
}

func TestListRun_VisibilityPaginatesPastPageWithoutMatch(t *testing.T) {
	const path = "/api/v1/user/repos"
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", path, pageQuery(1, 50)),
		httpmock.WithHeader(httpmock.StringResponse(http.StatusOK, reposJSON(1, 2)), "Link",
			`<https://app.copia.io/api/v1/user/repos?page=2&limit=50>; rel="next"`),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", path, pageQuery(2, 50)),
		httpmock.StringResponse(http.StatusOK, `[
			{"full_name":"my-org/repo-3","description":"","private":false,"updated_at":"2026-03-30T10:00:00Z"},
			{"full_name":"my-org/secret","description":"","private":true,"updated_at":"2026-03-30T10:00:00Z"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	err := ListRun(&ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      1,
		Visibility: "private",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "my-org/secret")
	require.Len(t, reg.Requests, 2)
}

func TestListRun_InvalidVisibility(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	ios, _, _, _ := iostreams.Test()

	err := ListRun(&ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
		Visibility: "internal",
	})
	require.Error(t, err)
	assert.Equal(t, `invalid visibility "internal": valid values are {public|private}`, err.Error())
	assert.Empty(t, reg.Requests)
}
