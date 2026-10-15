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

func TestListRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user/orgs"),
		httpmock.StringResponse(http.StatusOK, `[
			{"username":"my-org","full_name":"My Organization","description":"Industrial automation"},
			{"username":"other-org","full_name":"Other Org","description":"Testing"}
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
	assert.Contains(t, stdout.String(), "my-org")
	assert.Contains(t, stdout.String(), "other-org")
}

func TestListRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user/orgs"),
		httpmock.StringResponse(http.StatusOK, `[{"username":"my-org","full_name":"My Org","description":""}]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"username"}},
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"username"`)
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

func orgsJSON(first, count int) string {
	items := make([]string, count)
	for i := range items {
		items[i] = fmt.Sprintf(`{"username":"org-%d","full_name":"","description":""}`, first+i)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestListRun_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/user/orgs"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, orgsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 30)), httpmock.StringResponse(http.StatusOK, orgsJSON(51, 30)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      80,
	}

	err := ListRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 80, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "org-80")
	assert.Len(t, reg.Requests, 2)
}

func TestListRun_DefaultLimitCapsOutput(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/user/orgs", pageQuery(1, 30)),
		httpmock.StringResponse(http.StatusOK, orgsJSON(1, 30)),
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
	assert.Len(t, reg.Requests, 1)
}
