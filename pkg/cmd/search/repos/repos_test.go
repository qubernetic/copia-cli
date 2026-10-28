package repos

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

func TestSearchRepos_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/search"),
		httpmock.StringResponse(http.StatusOK, `{"data":[
			{"full_name":"my-org/plc-project","description":"PLC code","html_url":"https://app.copia.io/my-org/plc-project"},
			{"full_name":"other/hmi","description":"HMI config","html_url":"https://app.copia.io/other/hmi"}
		]}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Limit:      30,
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "plc-project")
}

func TestSearchRepos_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/search"),
		httpmock.StringResponse(http.StatusOK, `{"data":[
			{"full_name":"my-org/plc","description":"PLC","html_url":""}
		]}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"fullName"}},
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"full_name"`)
}

// searchJSON renders a search response wrapping count repositories numbered
// from first.
func searchJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"full_name":"my-org/plc-%d","description":"PLC %d","html_url":"https://app.copia.io/my-org/plc-%d"}`, first+i, first+i, first+i)
	}
	return `{"ok":true,"data":[` + strings.Join(parts, ",") + "]}"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestSearchRepos_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/search"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, searchJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, searchJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, searchJSON(101, 20)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Limit:      120,
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "PLC 1\n")
	assert.Contains(t, stdout.String(), "PLC 51\n")
	assert.Contains(t, stdout.String(), "PLC 120\n")

	require.Len(t, reg.Requests, 3)
	for _, req := range reg.Requests {
		assert.Equal(t, "plc", req.URL.Query().Get("q"))
	}
}

func TestSearchRepos_SortAndOrderOnEveryPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/search"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, searchJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 10)), httpmock.StringResponse(http.StatusOK, searchJSON(51, 10)))

	ios, _, stdout, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "plc",
		Sort:       "stars",
		Order:      "desc",
		Limit:      60,
	})
	require.NoError(t, err)
	assert.Equal(t, 60, strings.Count(stdout.String(), "\n"))

	require.Len(t, reg.Requests, 2)
	for _, req := range reg.Requests {
		q := req.URL.Query()
		assert.Equal(t, []string{"stars"}, q["sort"])
		assert.Equal(t, []string{"desc"}, q["order"])
		assert.Equal(t, "plc", q.Get("q"))
	}
}

func TestNewCmdSearchRepos_SortAndOrder(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantSort  []string
		wantOrder []string
	}{
		{
			name:      "sort defaults order to desc",
			args:      []string{"plc", "--sort", "stars"},
			wantSort:  []string{"stars"},
			wantOrder: []string{"desc"},
		},
		{
			name:      "order asc",
			args:      []string{"plc", "--sort", "updated", "--order", "asc"},
			wantSort:  []string{"updated"},
			wantOrder: []string{"asc"},
		},
		{
			name: "order without sort sends neither",
			args: []string{"plc", "--order", "asc"},
		},
		{
			name: "no flags sends neither",
			args: []string{"plc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", "/api/v1/repos/search"), httpmock.StringResponse(http.StatusOK, searchJSON(1, 1)))

			ios, _, stdout, _ := iostreams.Test()
			cmd := NewCmdSearchRepos(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"})
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, "my-org/plc-1  PLC 1\n", stdout.String())
			require.Len(t, reg.Requests, 1)
			q := reg.Requests[0].URL.Query()
			assert.Equal(t, tt.wantSort, q["sort"])
			assert.Equal(t, tt.wantOrder, q["order"])
		})
	}
}

func TestNewCmdSearchRepos_InvalidSortOrOrder(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "invalid sort",
			args:    []string{"plc", "--sort", "name"},
			wantErr: `invalid sort "name": valid values are {alpha|created|updated|size|stars|forks}`,
		},
		{
			name:    "invalid order",
			args:    []string{"plc", "--sort", "stars", "--order", "up"},
			wantErr: `invalid order "up": valid values are {asc|desc}`,
		},
		{
			name:    "invalid order without sort",
			args:    []string{"plc", "--order", "up"},
			wantErr: `invalid order "up": valid values are {asc|desc}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			cmd := NewCmdSearchRepos(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"})
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err)
			assert.EqualError(t, err, tt.wantErr)
			assert.Empty(t, reg.Requests)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestSearchRepos_InvalidLimit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{IO: ios, Limit: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")
}

func TestSearchRepos_Errors(t *testing.T) {
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
			respond: httpmock.StringResponse(http.StatusOK, `[not json`),
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
			reg.Register(httpmock.REST("GET", "/api/v1/repos/search"), tt.respond)

			ios, _, stdout, _ := iostreams.Test()
			err := SearchRun(&SearchOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Query:      "plc",
				Limit:      30,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestSearchRepos_EmptyResult(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/search"), httpmock.StringResponse(http.StatusOK, `{"ok":true,"data":[]}`))

	ios, _, stdout, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Query:      "nothing",
		Limit:      30,
	})
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}

func TestNewCmdSearchRepos_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/search", url.Values{"q": {"automation controller"}, "page": {"1"}, "limit": {"2"}}),
		httpmock.StringResponse(http.StatusOK, searchJSON(1, 2)),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	cmd := NewCmdSearchRepos(f)
	cmd.SetArgs([]string{"automation controller", "--limit", "2"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "my-org/plc-1  PLC 1\nmy-org/plc-2  PLC 2\n", stdout.String())
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdSearchRepos_Errors(t *testing.T) {
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
			},
			args:    []string{"plc"},
			wantErr: "no config",
		},
		{
			name:    "missing query",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{},
			wantErr: "accepts 1 arg(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_TOKEN", "")
			t.Setenv("COPIA_HOST", "")
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdSearchRepos(tt.factory)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}
