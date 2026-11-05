package comment

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const commentPath = "/api/v1/repos/my-org/my-repo/issues/12/comments"

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdComment(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdComment_Runs(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var posted string
	reg.Register(httpmock.REST("POST", commentPath), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		posted = string(b)
		return httpmock.StringResponse(http.StatusCreated, `{"id":1}`)(req)
	})

	ios, _, stdout, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12", "-b", "on it")
	require.NoError(t, err)
	assert.JSONEq(t, `{"body":"on it"}`, posted)
	assert.Equal(t, "Comment added to issue #12\n", stdout.String())
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdComment_InvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "abc", "--body", "x")
	require.EqualError(t, err, "invalid issue number: abc")
}

func TestNewCmdComment_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runCmd(t, f, "12", "--body", "x")
	require.EqualError(t, err, "no config")
}

func TestNewCmdComment_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "12", "--body", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}

func TestNewCmdComment_MissingBody(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "12")
	require.EqualError(t, err, "body required")
}

func runOpts(reg *httpmock.Registry) (*CommentOptions, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return &CommentOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Body:       "hello",
	}, stdout
}

func TestCommentRun_Rejected(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("POST", commentPath), httpmock.StringResponse(http.StatusOK, `{}`))

	opts, stdout := runOpts(reg)
	err := CommentRun(opts)
	require.EqualError(t, err, "failed to add comment (HTTP 200)")
	assert.Empty(t, stdout.String())
}

func TestCommentRun_TransportError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("POST", commentPath), func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})

	opts, _ := runOpts(reg)
	err := CommentRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestCommentRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _ := runOpts(reg)
	opts.Host = "bad\x7fhost"
	require.Error(t, CommentRun(opts))
	assert.Empty(t, reg.Requests)
}

func TestNewCmdComment_Web(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "long flag", args: []string{"12", "--web"}, wantURL: "https://app.copia.io/my-org/my-repo/issues/12"},
		{name: "short flag", args: []string{"12", "-w"}, wantURL: "https://app.copia.io/my-org/my-repo/issues/12"},
		{name: "repo flag", args: []string{"7", "-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/issues/7"},
		{
			name: "tty", args: []string{"12", "-w"}, tty: true,
			wantURL: "https://app.copia.io/my-org/my-repo/issues/12",
			wantErr: "Opening app.copia.io/my-org/my-repo/issues/12 in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_TOKEN", "")
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			repo := tt.repo
			if repo == "" {
				repo = "my-org/my-repo"
			}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: repo, Browser: b}

			require.NoError(t, runCmd(t, f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdComment_WebExclusive(t *testing.T) {
	for _, other := range [][]string{{"--body", "hi"}, {"--editor"}} {
		t.Run(other[0], func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			ios, _, _, _ := iostreams.Test()
			b := &browsertest.Browser{}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: b}

			err := runCmd(t, f, append([]string{"12", "--web"}, other...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "none of the others can be")
			assert.Empty(t, b.URLs)
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdComment_WebInvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	b := &browsertest.Browser{}
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo", Browser: b}
	require.EqualError(t, runCmd(t, f, "abc", "--web"), "invalid issue number: abc")
	assert.Empty(t, b.URLs)
}
