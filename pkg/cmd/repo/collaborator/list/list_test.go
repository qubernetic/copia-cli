package list

import (
	"errors"
	"fmt"
	"io"
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

const collaboratorsPath = "/api/v1/repos/my-org/my-repo/collaborators"

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdList(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

// usersJSON answers count collaborators user-<first> and on.
func usersJSON(first, count int) string {
	items := make([]string, count)
	for i := range items {
		n := first + i
		items[i] = fmt.Sprintf(`{"id":%d,"login":"user-%d","full_name":"User %d"}`, n, n, n)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams, limit int) *ListOptions {
	return &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Limit:      limit,
	}
}

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name string
		args []string
		tty  bool
		want string
	}{
		{
			name: "no terminal",
			want: "alice\tAlice A\nbob\t\n",
		},
		{
			name: "terminal", tty: true,
			want: "LOGIN  NAME\nalice  Alice A\nbob    \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(
				httpmock.QueryMatcher("GET", collaboratorsPath, pageQuery(1, 30)),
				httpmock.StringResponse(http.StatusOK, `[{"login":"alice","full_name":"Alice A"},{"login":"bob","full_name":""}]`),
			)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

			require.NoError(t, runCmd(f, tt.args...))
			assert.Equal(t, tt.want, stdout.String())
			assert.Empty(t, stderr.String())
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
	assert.Error(t, cmd.Args(cmd, []string{"extra"}))
}

func TestListRun_Limit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		pages [][3]int // page, page size asked, collaborators answered
		want  int
	}{
		{name: "default limit", limit: 30, pages: [][3]int{{1, 30, 30}}, want: 30},
		{name: "past one page", limit: 80, pages: [][3]int{{1, 50, 50}, {2, 30, 30}}, want: 80},
		{name: "fewer than the limit", limit: 80, pages: [][3]int{{1, 50, 50}, {2, 30, 4}}, want: 54},
		{name: "limit of one", limit: 1, pages: [][3]int{{1, 1, 1}}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			first := 1
			for _, p := range tt.pages {
				reg.Register(
					httpmock.QueryMatcher("GET", collaboratorsPath, pageQuery(p[0], p[1])),
					httpmock.StringResponse(http.StatusOK, usersJSON(first, p[2])),
				)
				first += p[2]
			}

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, ListRun(newOpts(reg, ios, tt.limit)))
			assert.Equal(t, tt.want, strings.Count(stdout.String(), "\n"))
			assert.Contains(t, stdout.String(), fmt.Sprintf("user-%d\tUser %d\n", tt.want, tt.want))
		})
	}
}

func TestListRun_InvalidLimit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := ListRun(&ListOptions{IO: ios, Limit: 0})
	assert.ErrorContains(t, err, "invalid limit")
}

func TestListRun_JSON(t *testing.T) {
	tests := []struct {
		name   string
		users  string
		fields []string
		jq     string
		want   string
	}{
		{
			name:   "all fields",
			users:  `[{"login":"alice","full_name":"Alice A","email":"alice@example.com"}]`,
			fields: []string{"login", "full_name"},
			want:   "[\n  {\n    \"full_name\": \"Alice A\",\n    \"login\": \"alice\"\n  }\n]\n",
		},
		{
			name:   "jq",
			users:  `[{"login":"alice","full_name":"Alice A"}]`,
			fields: []string{"login", "full_name"},
			jq:     `.[] | .login + " " + .full_name`,
			want:   "alice Alice A\n",
		},
		{
			name:   "no collaborators",
			users:  `[]`,
			fields: []string{"login"},
			want:   "[]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", collaboratorsPath), httpmock.StringResponse(http.StatusOK, tt.users))

			ios, _, stdout, stderr := iostreams.Test()
			opts := newOpts(reg, ios, 30)
			opts.JSON = cmdutil.JSONFlags{Fields: tt.fields, JQ: tt.jq}
			require.NoError(t, ListRun(opts))
			assert.Equal(t, tt.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestListRun_None(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", collaboratorsPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, ListRun(newOpts(reg, ios, 30)))
	assert.Empty(t, stdout.String())
	assert.Equal(t, "no collaborators found in my-org/my-repo\n", stderr.String())
}

func TestListRun_Errors(t *testing.T) {
	tests := []struct {
		name       string
		list    httpmock.Responder
		wantErr string
	}{
		{
			name:    "list refused with a message",
			list:    httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [read:repository]"}`),
			wantErr: "failed to list collaborators (HTTP 403): token does not have at least one of required scope(s): [read:repository]",
		},
		{
			name:    "list refused without a message",
			list:    httpmock.StringResponse(http.StatusNotFound, ``),
			wantErr: "failed to list collaborators (HTTP 404)",
		},
		{
			name:    "list transport error",
			list:    func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", collaboratorsPath), tt.list)

			ios, _, stdout, _ := iostreams.Test()
			err := ListRun(newOpts(reg, ios, 30))
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdList_RefusesPermissionField(t *testing.T) {
	// Copia has no route for one collaborator's permission.
	httpmock.ReplaceDefaultTransport(t, &httpmock.Registry{})
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	err := runCmd(f, "--json", "permission")
	assert.ErrorContains(t, err, `unknown JSON field: "permission"`)
}
