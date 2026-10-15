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
