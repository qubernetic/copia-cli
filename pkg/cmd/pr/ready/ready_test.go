package ready

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const prPath = "/api/v1/repos/my-org/my-repo/pulls/7"

func prJSON(title, state string) string {
	return fmt.Sprintf(`{"number":7,"title":%q,"state":%q}`, title, state)
}

func runOpts(reg *httpmock.Registry, undo bool) (*ReadyOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &ReadyOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Undo:       undo,
	}, stdout, stderr
}

func recordPatch(reg *httpmock.Registry) *string {
	var patched string
	reg.Register(httpmock.REST("PATCH", prPath), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		patched = string(b)
		return httpmock.StringResponse(http.StatusCreated, `{}`)(req)
	})
	return &patched
}

func failing(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

func TestReadyRun_MarksReady(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{title: "WIP: Fix the build", want: "Fix the build"},
		{title: "wip:Fix the build", want: "Fix the build"},
		{title: "Wip:  Fix the build", want: "Fix the build"},
		{title: "[WIP] Fix the build", want: "Fix the build"},
		{title: "[wip]Fix the build", want: "Fix the build"},
		{title: "WIP: [WIP] Fix the build", want: "Fix the build"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON(tt.title, "open")))
			patched := recordPatch(reg)

			opts, stdout, stderr := runOpts(reg, false)
			require.NoError(t, ReadyRun(opts))
			assert.JSONEq(t, fmt.Sprintf(`{"title":%q}`, tt.want), *patched)
			assert.Equal(t, `Pull request my-org/my-repo#7 (`+tt.want+`) is marked as "ready for review"`+"\n", stdout.String())
			assert.Empty(t, stderr.String())
			for _, r := range reg.Requests {
				assert.Equal(t, "token test-token", r.Header.Get("Authorization"))
			}
		})
	}
}

func TestReadyRun_Undo(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON("Fix the build", "open")))
	patched := recordPatch(reg)

	opts, stdout, stderr := runOpts(reg, true)
	require.NoError(t, ReadyRun(opts))
	assert.JSONEq(t, `{"title":"WIP: Fix the build"}`, *patched)
	assert.Equal(t, `Pull request my-org/my-repo#7 (WIP: Fix the build) is converted to "draft"`+"\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestReadyRun_NoOp(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		undo    bool
		wantErr string
	}{
		{name: "already ready", title: "Fix the build",
			wantErr: `! Pull request my-org/my-repo#7 (Fix the build) is already "ready for review"`},
		{name: "prefix lookalike is ready", title: "WIPE: Fix the build",
			wantErr: `! Pull request my-org/my-repo#7 (WIPE: Fix the build) is already "ready for review"`},
		{name: "already draft", title: "WIP: Fix the build", undo: true,
			wantErr: `! Pull request my-org/my-repo#7 (WIP: Fix the build) is already "in draft"`},
		{name: "already draft, bracket prefix any case", title: "[wip] Fix the build", undo: true,
			wantErr: `! Pull request my-org/my-repo#7 ([wip] Fix the build) is already "in draft"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON(tt.title, "open")))

			opts, stdout, stderr := runOpts(reg, tt.undo)
			require.NoError(t, ReadyRun(opts))
			reg.Verify(t)
			assert.Len(t, reg.Requests, 1, "no PATCH for a pull request in the wanted state")
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr+"\n", stderr.String())
		})
	}
}

func TestReadyRun_Closed(t *testing.T) {
	tests := []struct {
		undo    bool
		wantErr string
	}{
		{wantErr: `pull request my-org/my-repo#7 (WIP: Fix the build) is closed. Only draft pull requests can be marked as "ready for review"`},
		{undo: true, wantErr: `pull request my-org/my-repo#7 (WIP: Fix the build) is closed. Only open pull requests can be converted to "draft"`},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON("WIP: Fix the build", "closed")))

		opts, stdout, _ := runOpts(reg, tt.undo)
		require.EqualError(t, ReadyRun(opts), tt.wantErr)
		reg.Verify(t)
		assert.Len(t, reg.Requests, 1, "no PATCH for a closed pull request")
		assert.Empty(t, stdout.String())
	}
}

func TestReadyRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
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
			name: "title is only a prefix",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON("WIP: ", "open")))
			},
			wantErr: "pull request my-org/my-repo#7 (WIP: ) has no title without its WIP prefix; set one with `pr edit --title`",
		},
		{
			name: "update rejected with message",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON("WIP: x", "open")))
				reg.Register(httpmock.REST("PATCH", prPath),
					httpmock.StringResponse(http.StatusForbidden, `{"message":"must be the poster"}`))
			},
			wantErr: "failed to update pull request my-org/my-repo#7 (HTTP 403): must be the poster",
		},
		{
			name: "update rejected",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON("WIP: x", "open")))
				reg.Register(httpmock.REST("PATCH", prPath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "failed to update pull request my-org/my-repo#7 (HTTP 404)",
		},
		{
			name: "update transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON("WIP: x", "open")))
				reg.Register(httpmock.REST("PATCH", prPath), failing)
			},
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)

			opts, stdout, _ := runOpts(reg, false)
			err := ReadyRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestReadyRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _, _ := runOpts(reg, false)
	opts.Host = "bad\x7fhost"
	require.Error(t, ReadyRun(opts))
	assert.Empty(t, reg.Requests)
}

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdReady(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdReady_Runs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		title   string
		wantJ   string
		wantOut string
	}{
		{name: "number", args: []string{"7"}, title: "WIP: x", wantJ: `{"title":"x"}`,
			wantOut: `Pull request my-org/my-repo#7 (x) is marked as "ready for review"`},
		{name: "hash number", args: []string{"#7"}, title: "[WIP] x", wantJ: `{"title":"x"}`,
			wantOut: `Pull request my-org/my-repo#7 (x) is marked as "ready for review"`},
		{name: "--undo", args: []string{"7", "--undo"}, title: "x", wantJ: `{"title":"WIP: x"}`,
			wantOut: `Pull request my-org/my-repo#7 (WIP: x) is converted to "draft"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, prJSON(tt.title, "open")))
			patched := recordPatch(reg)

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runCmd(t, readyFactory(ios), tt.args...))
			assert.JSONEq(t, tt.wantJ, *patched)
			assert.Equal(t, tt.wantOut+"\n", stdout.String())
		})
	}
}

func TestNewCmdReady_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/thing/pulls/5"),
		httpmock.StringResponse(http.StatusOK, `{"number":5,"title":"WIP: x","state":"open"}`))
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/other/thing/pulls/5"),
		httpmock.StringResponse(http.StatusCreated, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "https://app.copia.io/other/thing/pulls/5"))
	assert.Equal(t, `Pull request other/thing#5 (x) is marked as "ready for review"`+"\n", stdout.String())
}

func TestNewCmdReady_URLOnOtherHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "https://copia.example.com/other/thing/pulls/5")
	require.EqualError(t, err, "pull request URL host copia.example.com does not match the active host app.copia.io")
}

func TestNewCmdReady_InvalidArgument(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, readyFactory(ios), "abc"), "invalid pull request number or URL: abc")
	require.Error(t, runCmd(t, readyFactory(ios)))
}

func TestNewCmdReady_AuthFailure(t *testing.T) {
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

func TestNewCmdReady_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
