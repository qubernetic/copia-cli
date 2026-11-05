package updatebranch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	prPath      = "/api/v1/repos/my-org/my-repo/pulls/7"
	updatePath  = "/api/v1/repos/my-org/my-repo/pulls/7/update"
	comparePath = "/api/v1/repos/my-org/my-repo/compare/feature...main"
)

func prJSON(state string, merged bool, headRepo string) string {
	return fmt.Sprintf(`{"number":7,"title":"Fix the build","state":%q,"merged":%t,`+
		`"head":{"ref":"feature","repo":{"full_name":%q}},`+
		`"base":{"ref":"main","repo":{"full_name":"my-org/my-repo"}}}`, state, merged, headRepo)
}

var openPR = prJSON("open", false, "my-org/my-repo")

func behind(n int) httpmock.Responder {
	return httpmock.StringResponse(http.StatusOK, fmt.Sprintf(`{"total_commits":%d,"commits":[]}`, n))
}

func runOpts(reg *httpmock.Registry, rebase bool) (*UpdateBranchOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &UpdateBranchOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Rebase:     rebase,
	}, stdout, stderr
}

func failing(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

func TestUpdateBranchRun_Styles(t *testing.T) {
	for _, tt := range []struct {
		rebase    bool
		wantStyle string
	}{
		{rebase: false, wantStyle: "merge"},
		{rebase: true, wantStyle: "rebase"},
	} {
		t.Run(tt.wantStyle, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, openPR))
			reg.Register(httpmock.REST("GET", comparePath), behind(2))
			reg.Register(httpmock.QueryMatcher("POST", updatePath, url.Values{"style": {tt.wantStyle}}),
				httpmock.StringResponse(http.StatusOK, ``))

			opts, stdout, stderr := runOpts(reg, tt.rebase)
			require.NoError(t, UpdateBranchRun(opts))
			assert.Equal(t, "Updated the branch of pull request my-org/my-repo#7 (Fix the build)\n", stdout.String())
			assert.Empty(t, stderr.String())
			for _, r := range reg.Requests {
				assert.Equal(t, "token test-token", r.Header.Get("Authorization"))
			}
		})
	}
}

func TestUpdateBranchRun_AlreadyUpToDate(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, openPR))
	reg.Register(httpmock.REST("GET", comparePath), behind(0))

	opts, stdout, stderr := runOpts(reg, false)
	require.NoError(t, UpdateBranchRun(opts))
	assert.Len(t, reg.Requests, 2, "no update for an up-to-date branch")
	assert.Empty(t, stdout.String())
	assert.Equal(t, "! The branch of pull request my-org/my-repo#7 (Fix the build) is already up to date\n", stderr.String())
}

func TestUpdateBranchRun_CompareSkipped(t *testing.T) {
	tests := []struct {
		name    string
		pr      string
		compare httpmock.Responder
	}{
		{name: "head in a fork", pr: prJSON("open", false, "someone/my-repo")},
		{name: "compare refused", pr: openPR, compare: httpmock.StringResponse(http.StatusNotFound, `{}`)},
		{name: "compare transport error", pr: openPR, compare: failing},
		{name: "compare bad json", pr: openPR, compare: httpmock.StringResponse(http.StatusOK, `{`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, tt.pr))
			if tt.compare != nil {
				reg.Register(httpmock.REST("GET", comparePath), tt.compare)
			}
			reg.Register(httpmock.REST("POST", updatePath), httpmock.StringResponse(http.StatusOK, ``))

			opts, stdout, _ := runOpts(reg, false)
			require.NoError(t, UpdateBranchRun(opts))
			assert.Equal(t, "Updated the branch of pull request my-org/my-repo#7 (Fix the build)\n", stdout.String())
		})
	}
}

func TestUpdateBranchRun_EscapesBranchNames(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	pr := `{"number":7,"title":"x","state":"open",` +
		`"head":{"ref":"feat/a#1","repo":{"full_name":"my-org/my-repo"}},` +
		`"base":{"ref":"release/1.0","repo":{"full_name":"my-org/my-repo"}}}`
	reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, pr))
	reg.Register(httpmock.REST("GET", "/compare/feat/a#1...release/1.0"), behind(0))

	opts, _, stderr := runOpts(reg, false)
	require.NoError(t, UpdateBranchRun(opts))
	assert.Equal(t, "/api/v1/repos/my-org/my-repo/compare/feat/a%231...release/1.0", reg.Requests[1].URL.EscapedPath())
	assert.Contains(t, stderr.String(), "is already up to date")
}

func TestUpdateBranchRun_NotOpen(t *testing.T) {
	tests := []struct {
		name    string
		pr      string
		wantErr string
	}{
		{name: "merged", pr: prJSON("closed", true, "my-org/my-repo"),
			wantErr: "pull request my-org/my-repo#7 (Fix the build) can't be updated because it was already merged"},
		{name: "closed", pr: prJSON("closed", false, "my-org/my-repo"),
			wantErr: "pull request my-org/my-repo#7 (Fix the build) can't be updated because it is closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, tt.pr))

			opts, stdout, _ := runOpts(reg, false)
			require.EqualError(t, UpdateBranchRun(opts), tt.wantErr)
			assert.Len(t, reg.Requests, 1)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestUpdateBranchRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		rebase  bool
		update  bool
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name: "pull request not found",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "pull request my-org/my-repo#7 not found",
		},
		{
			name: "lookup rejected",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to read pull request my-org/my-repo#7 (HTTP 403)",
		},
		{
			name: "lookup transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), failing)
			},
			wantErr: "boom",
		},
		{
			name: "lookup bad json",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, `{`))
			},
			wantErr: "unexpected end of JSON input",
		},
		{
			name:   "merge conflict",
			update: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", updatePath),
					httpmock.StringResponse(http.StatusConflict, `{"message":"merge failed because of conflict"}`))
			},
			wantErr: "failed to update the branch of pull request my-org/my-repo#7 (HTTP 409): merge failed because of conflict",
		},
		{
			name:   "rebase conflict",
			update: true,
			rebase: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", updatePath),
					httpmock.StringResponse(http.StatusConflict, `{"message":"rebase failed because of conflict"}`))
			},
			wantErr: "failed to update the branch of pull request my-org/my-repo#7 (HTTP 409): rebase failed because of conflict",
		},
		{
			name:   "validation error",
			update: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", updatePath),
					httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"head branch does not exist"}`))
			},
			wantErr: "failed to update the branch of pull request my-org/my-repo#7 (HTTP 422): head branch does not exist",
		},
		{
			name:   "forbidden without message",
			update: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", updatePath), httpmock.StringResponse(http.StatusForbidden, ``))
			},
			wantErr: "failed to update the branch of pull request my-org/my-repo#7 (HTTP 403)",
		},
		{
			name:   "update transport error",
			update: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", updatePath), failing)
			},
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.update {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, openPR))
				reg.Register(httpmock.REST("GET", comparePath), behind(1))
			}
			tt.stubs(reg)

			opts, stdout, _ := runOpts(reg, tt.rebase)
			err := UpdateBranchRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestUpdateBranchRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _, _ := runOpts(reg, false)
	opts.Host = "bad\x7fhost"
	require.Error(t, UpdateBranchRun(opts))
	assert.Empty(t, reg.Requests)
}

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdUpdateBranch(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func updateFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdUpdateBranch_Runs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantStyle string
	}{
		{name: "number", args: []string{"7"}, wantStyle: "merge"},
		{name: "hash number", args: []string{"#7"}, wantStyle: "merge"},
		{name: "--rebase", args: []string{"7", "--rebase"}, wantStyle: "rebase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, openPR))
			reg.Register(httpmock.REST("GET", comparePath), behind(1))
			reg.Register(httpmock.QueryMatcher("POST", updatePath, url.Values{"style": {tt.wantStyle}}),
				httpmock.StringResponse(http.StatusOK, ``))

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runCmd(t, updateFactory(ios), tt.args...))
			assert.Equal(t, "Updated the branch of pull request my-org/my-repo#7 (Fix the build)\n", stdout.String())
		})
	}
}

func TestNewCmdUpdateBranch_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/thing/pulls/5"),
		httpmock.StringResponse(http.StatusOK, `{"number":5,"title":"x","state":"open",`+
			`"head":{"ref":"feature","repo":{"full_name":"other/thing"}},`+
			`"base":{"ref":"main","repo":{"full_name":"other/thing"}}}`))
	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/thing/compare/feature...main"), behind(1))
	reg.Register(httpmock.REST("POST", "/api/v1/repos/other/thing/pulls/5/update"),
		httpmock.StringResponse(http.StatusOK, ``))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, updateFactory(ios), "https://app.copia.io/other/thing/pulls/5"))
	assert.Equal(t, "Updated the branch of pull request other/thing#5 (x)\n", stdout.String())
}

func TestNewCmdUpdateBranch_URLOnOtherHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, updateFactory(ios), "https://copia.example.com/other/thing/pulls/5")
	require.EqualError(t, err, "pull request URL host copia.example.com does not match the active host app.copia.io")
}

func TestNewCmdUpdateBranch_InvalidArgument(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, updateFactory(ios), "abc"), "invalid pull request number or URL: abc")
	require.Error(t, runCmd(t, updateFactory(ios)))
}

func TestNewCmdUpdateBranch_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	require.EqualError(t, runCmd(t, f, "7"), "no config")
}

func TestNewCmdUpdateBranch_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
