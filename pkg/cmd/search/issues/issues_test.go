package issues

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

func TestSearchIssues_DefaultStateAll(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/plc/issues"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":5,"title":"Closed issue","state":"closed"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "plc",
		Query:      "issue",
		Limit:      30,
		// State intentionally empty — should default to "all"
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Closed issue")
}

func TestSearchIssues_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/plc/issues"),
		httpmock.StringResponse(http.StatusOK, `[
			{"number":12,"title":"Fix PLC timeout","state":"open"},
			{"number":5,"title":"Sensor error","state":"closed"}
		]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "plc",
		Query:      "timeout",
		State:      "open",
		Limit:      30,
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Fix PLC timeout")
}

// issuesJSON renders count issues numbered from first.
func issuesJSON(first, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"number":%d,"title":"Timeout %d","state":"open"}`, first+i, first+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestSearchIssues_PaginatesPastPageCap(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/plc/issues"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, issuesJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, issuesJSON(51, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(3, 20)), httpmock.StringResponse(http.StatusOK, issuesJSON(101, 20)))

	ios, _, stdout, _ := iostreams.Test()

	opts := &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "plc",
		Query:      "sensor timeout",
		State:      "open",
		Limit:      120,
	}

	err := SearchRun(opts)
	require.NoError(t, err)
	assert.Equal(t, 120, strings.Count(stdout.String(), "\n"))
	assert.Contains(t, stdout.String(), "#120")

	require.Len(t, reg.Requests, 3)
	for _, req := range reg.Requests {
		q := req.URL.Query()
		assert.Equal(t, "sensor timeout", q.Get("q"))
		assert.Equal(t, "open", q.Get("state"))
		assert.Equal(t, "issues", q.Get("type"))
	}
}

func TestSearchIssues_LabelsOnEveryPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	path := "/api/v1/repos/my-org/plc/issues"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, issuesJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 10)), httpmock.StringResponse(http.StatusOK, issuesJSON(51, 10)))

	ios, _, stdout, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "plc",
		Query:      "bug",
		Labels:     []string{"a", "b"},
		Limit:      60,
	})
	require.NoError(t, err)
	assert.Equal(t, 60, strings.Count(stdout.String(), "\n"))

	require.Len(t, reg.Requests, 2)
	for _, req := range reg.Requests {
		assert.Equal(t, []string{"a,b"}, req.URL.Query()["labels"])
		assert.Equal(t, "bug", req.URL.Query().Get("q"))
	}
}

func TestSearchIssues_NoLabelsParameterWithoutLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/plc/issues"),
		httpmock.StringResponse(http.StatusOK, issuesJSON(1, 1)),
	)

	ios, _, _, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "plc",
		Query:      "bug",
		Limit:      30,
	})
	require.NoError(t, err)
	require.Len(t, reg.Requests, 1)
	assert.NotContains(t, reg.Requests[0].URL.Query(), "labels")
}

func TestSearchIssues_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/plc/issues"),
		httpmock.StringResponse(http.StatusOK, `[{"number":12,"title":"Fix PLC timeout","state":"open"}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "plc",
		Query:      "timeout",
		Limit:      30,
		JSON:       cmdutil.JSONFlags{Fields: []string{"number", "title"}},
	})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"number": 12`)
	assert.Contains(t, stdout.String(), `"title": "Fix PLC timeout"`)
}

func TestSearchIssues_InvalidLimit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := SearchRun(&SearchOptions{IO: ios, Limit: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")
}

func TestSearchIssues_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "not found",
			respond: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "HTTP 404",
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
			reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/plc/issues"), tt.respond)

			ios, _, stdout, _ := iostreams.Test()
			err := SearchRun(&SearchOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "plc",
				Query:      "timeout",
				Limit:      30,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdSearchIssues_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/plc/issues", url.Values{
			"q": {"sensor"}, "state": {"closed"}, "type": {"issues"}, "page": {"1"}, "limit": {"5"},
		}),
		httpmock.StringResponse(http.StatusOK, `[{"number":5,"title":"Sensor error","state":"closed"}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/plc"}
	cmd := NewCmdSearchIssues(f)
	cmd.SetArgs([]string{"sensor", "--state", "closed", "-L", "5"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "#5  Sensor error  closed\n", stdout.String())
}

func TestNewCmdSearchIssues_LabelFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/repos/my-org/plc/issues", url.Values{
			"q": {"bug"}, "labels": {"a,b,c"}, "page": {"1"}, "limit": {"30"},
		}),
		httpmock.StringResponse(http.StatusOK, `[{"number":7,"title":"Bug","state":"open"}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/plc"}
	cmd := NewCmdSearchIssues(f)
	cmd.SetArgs([]string{"bug", "--label", "a", "-l", "b,c"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "#7  Bug  open\n", stdout.String())
}

func TestNewCmdSearchIssues_Errors(t *testing.T) {
	noConfig := func() (*config.Config, error) { return nil, errors.New("no config") }

	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name: "auth failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Config: noConfig, Repo: "my-org/plc"}
			},
			args:    []string{"sensor"},
			wantErr: "no config",
		},
		{
			name: "no repo context",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			args:    []string{"sensor"},
			wantErr: "could not determine repository",
		},
		{
			name: "missing query",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/plc"}
			},
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
			cmd := NewCmdSearchIssues(tt.factory(ios))
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

func TestNewCmdSearchIssues_JSONFieldsAndJQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/plc/issues"),
		httpmock.StringResponse(http.StatusOK, `[{"number":5,"title":"Sensor error","state":"closed"}]`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/plc"}
	cmd := NewCmdSearchIssues(f)
	cmd.SetArgs([]string{"sensor", "--json", "number,title", "--jq", ".[] | \"#\\(.number) \\(.title)\""})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "#5 Sensor error\n", stdout.String())
}
