package prs

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const searchPath = "/api/v1/repos/issues/search"

const twoPRs = `[
	{"number":12,"title":"Fix PLC timeout","state":"open","html_url":"https://app.copia.io/my-org/plc/pulls/12",
	 "user":{"login":"alice"},"labels":[{"name":"bug"}],"repository":{"full_name":"my-org/plc"},
	 "pull_request":{"merged":false},"created_at":"2026-11-01T10:00:00Z","updated_at":"2026-11-02T10:00:00Z"},
	{"number":5,"title":"Add sensor","state":"closed","html_url":"https://app.copia.io/my-org/hmi/pulls/5",
	 "user":{"login":"bob"},"labels":[],"repository":{"full_name":"my-org/hmi"},
	 "pull_request":{"merged":true},"created_at":"2026-10-01T10:00:00Z","updated_at":"2026-10-02T10:00:00Z"}
]`

func newOpts(reg *httpmock.Registry) (*SearchOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
	}, stdout
}

func TestSearchPRs_Filters(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*SearchOptions)
		want  url.Values
	}{
		{
			name:  "query only searches every state",
			setup: func(o *SearchOptions) { o.Query = "timeout" },
			want:  url.Values{"type": {"pulls"}, "state": {"all"}, "q": {"timeout"}},
		},
		{
			name:  "state",
			setup: func(o *SearchOptions) { o.State = "closed" },
			want:  url.Values{"type": {"pulls"}, "state": {"closed"}},
		},
		{
			name:  "labels",
			setup: func(o *SearchOptions) { o.Labels = []string{"bug", "plc"} },
			want:  url.Values{"type": {"pulls"}, "state": {"all"}, "labels": {"bug,plc"}},
		},
		{
			name:  "assignee @me",
			setup: func(o *SearchOptions) { o.Assignee = "@me" },
			want:  url.Values{"type": {"pulls"}, "state": {"all"}, "assigned": {"true"}},
		},
		{
			name:  "owner",
			setup: func(o *SearchOptions) { o.Owner = "my-org" },
			want:  url.Values{"type": {"pulls"}, "state": {"all"}, "owner": {"my-org"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			want := tt.want
			want.Set("page", "1")
			want.Set("limit", "30")
			reg.Register(httpmock.QueryMatcher("GET", searchPath, want), httpmock.StringResponse(http.StatusOK, `[]`))

			opts, _ := newOpts(reg)
			tt.setup(opts)
			require.NoError(t, SearchRun(opts))
			assert.Equal(t, want, reg.Requests[0].URL.Query())
		})
	}
}

func TestSearchPRs_Author(t *testing.T) {
	tests := []struct {
		name   string
		author string
		me     bool
		want   url.Values
		out    string
	}{
		{
			name:   "login is sent and checked on each result",
			author: "Bob",
			want:   url.Values{"created_by": {"Bob"}, "limit": {"50"}},
			out:    "my-org/hmi  #5  Add sensor  merged\n",
		},
		{
			name:   "@me is sent and checked as the authenticated user",
			author: "@me",
			me:     true,
			want:   url.Values{"created": {"true"}, "created_by": nil, "limit": {"50"}},
			out:    "my-org/plc  #12  Fix PLC timeout  open\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.me {
				reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"alice"}`))
			}
			reg.Register(httpmock.QueryMatcher("GET", searchPath, tt.want), httpmock.StringResponse(http.StatusOK, twoPRs))

			opts, stdout := newOpts(reg)
			opts.Author = tt.author
			require.NoError(t, SearchRun(opts))
			assert.Equal(t, tt.out, stdout.String())
		})
	}

	t.Run("@me and --assignee read the user once", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"alice"}`))
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"created": {"true"}, "assigned": {"true"}}),
			httpmock.StringResponse(http.StatusOK, `[]`))

		opts, _ := newOpts(reg)
		opts.Author = "@me"
		opts.Assignee = "alice"
		require.NoError(t, SearchRun(opts))
		assert.Len(t, reg.Requests, 2)
	})

	t.Run("@me lookup fails", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusUnauthorized, `{}`))

		opts, _ := newOpts(reg)
		opts.Author = "@me"
		assert.EqualError(t, SearchRun(opts), "failed to resolve @me (HTTP 401)")
	})
}

func TestSearchPRs_Table(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", searchPath), httpmock.StringResponse(http.StatusOK, twoPRs))

	opts, stdout := newOpts(reg)
	opts.Query = "x"
	require.NoError(t, SearchRun(opts))
	assert.Equal(t, "my-org/plc  #12  Fix PLC timeout  open\nmy-org/hmi  #5   Add sensor       merged\n", stdout.String())
}

func TestSearchPRs_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", searchPath), httpmock.StringResponse(http.StatusOK, twoPRs))

	opts, stdout := newOpts(reg)
	opts.Query = "x"
	opts.JSON = cmdutil.JSONFlags{Fields: []string{"number", "state", "repository", "user", "labels", "html_url"}}
	require.NoError(t, SearchRun(opts))
	assert.JSONEq(t, `[
		{"number":12,"state":"open","repository":{"full_name":"my-org/plc"},"user":{"login":"alice"},
		 "labels":[{"name":"bug"}],"html_url":"https://app.copia.io/my-org/plc/pulls/12"},
		{"number":5,"state":"merged","repository":{"full_name":"my-org/hmi"},"user":{"login":"bob"},
		 "labels":[],"html_url":"https://app.copia.io/my-org/hmi/pulls/5"}
	]`, stdout.String())
}

func TestSearchPRs_Paging(t *testing.T) {
	page := func(from, n int) string {
		out := "["
		for i := 0; i < n; i++ {
			if i > 0 {
				out += ","
			}
			out += `{"number":` + strconv.Itoa(from+i) + `,"title":"t","state":"open","repository":{"full_name":"o/r"}}`
		}
		return out + "]"
	}

	t.Run("fetches pages until the limit", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"page": {"1"}, "limit": {"50"}}),
			httpmock.StringResponse(http.StatusOK, page(1, 50)))
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"page": {"2"}, "limit": {"10"}}),
			httpmock.StringResponse(http.StatusOK, page(51, 10)))

		opts, stdout := newOpts(reg)
		opts.Query = "t"
		opts.Limit = 60
		opts.JSON = cmdutil.JSONFlags{Fields: []string{"number"}, JQ: "length"}
		require.NoError(t, SearchRun(opts))
		assert.Equal(t, "60\n", stdout.String())
		for _, req := range reg.Requests {
			assert.Equal(t, "pulls", req.URL.Query().Get("type"))
			assert.Equal(t, "t", req.URL.Query().Get("q"))
		}
	})

	t.Run("stops at a short page", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"page": {"1"}, "limit": {"50"}}),
			httpmock.StringResponse(http.StatusOK, page(1, 3)))

		opts, stdout := newOpts(reg)
		opts.Query = "t"
		opts.Limit = 100
		opts.JSON = cmdutil.JSONFlags{Fields: []string{"number"}, JQ: "length"}
		require.NoError(t, SearchRun(opts))
		assert.Equal(t, "3\n", stdout.String())
	})
}

func TestSearchPRs_Repo(t *testing.T) {
	t.Run("narrows by owner and keeps the repository", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"owner": {"my-org"}, "limit": {"50"}}),
			httpmock.StringResponse(http.StatusOK, twoPRs))

		opts, stdout := newOpts(reg)
		opts.Query = "x"
		opts.Repo = "my-org/HMI"
		require.NoError(t, SearchRun(opts))
		assert.Equal(t, "my-org/hmi  #5  Add sensor  merged\n", stdout.String())
	})

	t.Run("conflicts with another owner", func(t *testing.T) {
		reg := &httpmock.Registry{}
		opts, _ := newOpts(reg)
		opts.Repo = "my-org/plc"
		opts.Owner = "other"
		err := SearchRun(opts)
		assert.EqualError(t, err, `--owner "other" does not match the owner of --repo "my-org/plc"`)
		assert.Empty(t, reg.Requests)
	})

	t.Run("invalid", func(t *testing.T) {
		reg := &httpmock.Registry{}
		opts, _ := newOpts(reg)
		opts.Repo = "plc"
		assert.EqualError(t, SearchRun(opts), `expected owner/repo format, got "plc"`)
		assert.Empty(t, reg.Requests)
	})
}

func TestSearchPRs_AssigneeLogin(t *testing.T) {
	t.Run("the authenticated user", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"Alice"}`))
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"assigned": {"true"}}),
			httpmock.StringResponse(http.StatusOK, `[]`))

		opts, _ := newOpts(reg)
		opts.Assignee = "alice"
		require.NoError(t, SearchRun(opts))
	})

	t.Run("another user", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"alice"}`))

		opts, _ := newOpts(reg)
		opts.Assignee = "bob"
		err := SearchRun(opts)
		assert.EqualError(t, err, `--assignee "bob": the search endpoint filters only on the authenticated user ("alice"), use @me`)
		assert.Len(t, reg.Requests, 1)
	})

	t.Run("user lookup fails", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusUnauthorized, `{}`))

		opts, _ := newOpts(reg)
		opts.Assignee = "bob"
		assert.EqualError(t, SearchRun(opts), "failed to resolve @me (HTTP 401)")
	})
}

func TestSearchPRs_Errors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*SearchOptions)
		status  int
		wantErr string
	}{
		{"unknown author", func(o *SearchOptions) { o.Author = "ghost" }, http.StatusNotFound, `no user named "ghost"`},
		{"unknown owner", func(o *SearchOptions) { o.Owner = "ghost" }, http.StatusBadRequest, `no user or organization named "ghost"`},
		{"other error", func(o *SearchOptions) { o.Query = "x" }, http.StatusInternalServerError, "API error (HTTP 500)"},
		{"404 without author", func(o *SearchOptions) { o.Query = "x" }, http.StatusNotFound, "API error (HTTP 404)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", searchPath), httpmock.StringResponse(tt.status, `{}`))
			opts, _ := newOpts(reg)
			tt.setup(opts)
			assert.EqualError(t, SearchRun(opts), tt.wantErr)
		})
	}
}

func TestSearchPRs_Validation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*SearchOptions)
		wantErr string
	}{
		{"state", func(o *SearchOptions) { o.State = "merged" }, `invalid state "merged": use open or closed`},
		{"limit", func(o *SearchOptions) { o.Limit = 0 }, "invalid limit: 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			opts, _ := newOpts(reg)
			tt.setup(opts)
			assert.EqualError(t, SearchRun(opts), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}

func newTestFactory(t *testing.T, reg *httpmock.Registry) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, stdout, _ := iostreams.Test()
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}, stdout
}

func TestNewCmdSearchPRs(t *testing.T) {
	t.Run("flags reach the request", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{
			"q": {"timeout"}, "type": {"pulls"}, "state": {"open"}, "labels": {"bug,plc"},
			"created_by": {"bob"}, "assigned": {"true"}, "owner": {"my-org"}, "page": {"1"}, "limit": {"50"},
		}), httpmock.StringResponse(http.StatusOK, twoPRs))

		f, stdout := newTestFactory(t, reg)
		cmd := NewCmdSearchPRs(f)
		cmd.SetArgs([]string{"timeout", "-s", "open", "-l", "bug", "-l", "plc", "-L", "5",
			"--author", "bob", "--assignee", "@me", "--owner", "my-org", "--json", "number", "-q", ".[0].number"})
		cmd.SetOut(&bytes.Buffer{})
		require.NoError(t, cmd.Execute())
		assert.Equal(t, "5\n", stdout.String())
	})

	t.Run("flags without a query", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"q": nil, "state": {"closed"}}),
			httpmock.StringResponse(http.StatusOK, `[]`))

		f, _ := newTestFactory(t, reg)
		cmd := NewCmdSearchPRs(f)
		cmd.SetArgs([]string{"--state", "closed"})
		require.NoError(t, cmd.Execute())
		assert.NotContains(t, reg.Requests[0].URL.Query(), "q")
	})

	t.Run("-R from the factory", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.QueryMatcher("GET", searchPath, url.Values{"owner": {"my-org"}}),
			httpmock.StringResponse(http.StatusOK, twoPRs))

		f, stdout := newTestFactory(t, reg)
		f.Repo = "my-org/plc"
		cmd := NewCmdSearchPRs(f)
		cmd.SetArgs([]string{"timeout"})
		require.NoError(t, cmd.Execute())
		assert.Equal(t, "my-org/plc  #12  Fix PLC timeout  open\n", stdout.String())
	})

	t.Run("no query and no flags", func(t *testing.T) {
		reg := &httpmock.Registry{}
		f, _ := newTestFactory(t, reg)
		cmd := NewCmdSearchPRs(f)
		cmd.SetArgs([]string{})
		cmd.SetErr(&bytes.Buffer{})
		assert.EqualError(t, cmd.Execute(), "specify search keywords or flags")
		assert.Empty(t, reg.Requests)
	})

	t.Run("not logged in", func(t *testing.T) {
		reg := &httpmock.Registry{}
		f, _ := newTestFactory(t, reg)
		f.Host, f.Token = "", ""
		f.Config = func() (*config.Config, error) { return nil, errors.New("no config") }
		cmd := NewCmdSearchPRs(f)
		cmd.SetArgs([]string{"x"})
		cmd.SetErr(&bytes.Buffer{})
		assert.Error(t, cmd.Execute())
		assert.Empty(t, reg.Requests)
	})
}
