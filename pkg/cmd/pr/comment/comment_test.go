package comment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	commentPath = "/api/v1/repos/my-org/my-repo/issues/7/comments"
	commentURL  = "https://app.copia.io/my-org/my-repo/pulls/7#issuecomment-42"
	created     = `{"id":42,"html_url":"` + commentURL + `","body":"Looks good"}`
)

func runOpts(reg *httpmock.Registry, body string) (*CommentOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &CommentOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Body:       body,
	}, stdout, stderr
}

func failing(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }

func TestCommentRun_Success(t *testing.T) {
	tests := []struct {
		name    string
		resp    string
		wantOut string
	}{
		{name: "prints the comment URL", resp: created, wantOut: commentURL + "\n"},
		{name: "no URL in the response", resp: `{"id":42}`, wantOut: "Added comment to pull request my-org/my-repo#7\n"},
		{name: "unreadable response", resp: `{`, wantOut: "Added comment to pull request my-org/my-repo#7\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			var posted string
			reg.Register(httpmock.REST("POST", commentPath), func(req *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(req.Body)
				posted = string(b)
				return httpmock.StringResponse(http.StatusCreated, tt.resp)(req)
			})

			opts, stdout, stderr := runOpts(reg, "Looks good")
			require.NoError(t, CommentRun(opts))
			assert.JSONEq(t, `{"body":"Looks good"}`, posted)
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, stderr.String())
			require.Len(t, reg.Requests, 1)
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
			assert.Equal(t, "application/json", reg.Requests[0].Header.Get("Content-Type"))
		})
	}
}

func TestCommentRun_BlankBody(t *testing.T) {
	for _, body := range []string{"", "  \n\t"} {
		reg := &httpmock.Registry{}
		opts, stdout, _ := runOpts(reg, body)
		require.EqualError(t, CommentRun(opts), "body cannot be blank")
		assert.Empty(t, reg.Requests, "no request for a blank body")
		assert.Empty(t, stdout.String())
	}
}

func TestCommentRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		stub    httpmock.Responder
		wantErr string
	}{
		{
			name:    "pull request not found",
			stub:    httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "pull request my-org/my-repo#7 not found",
		},
		{
			name:    "rejected with message",
			stub:    httpmock.StringResponse(http.StatusForbidden, `{"message":"user is blocked"}`),
			wantErr: "failed to add comment to pull request my-org/my-repo#7 (HTTP 403): user is blocked",
		},
		{
			name:    "rejected",
			stub:    httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "failed to add comment to pull request my-org/my-repo#7 (HTTP 500)",
		},
		{
			name:    "transport error",
			stub:    failing,
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("POST", commentPath), tt.stub)

			opts, stdout, _ := runOpts(reg, "Looks good")
			err := CommentRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestCommentRun_InvalidHost(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, _, _ := runOpts(reg, "Looks good")
	opts.Host = "bad\x7fhost"
	require.Error(t, CommentRun(opts))
	assert.Empty(t, reg.Requests)
}

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

func TestNewCmdComment_Body(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "comment.md")
	require.NoError(t, os.WriteFile(bodyFile, []byte("From a file\n"), 0o600))

	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantBody string
	}{
		{name: "--body", args: []string{"7", "--body", "Looks good"}, wantBody: "Looks good"},
		{name: "-b", args: []string{"7", "-b", "Looks good"}, wantBody: "Looks good"},
		{name: "--body-file", args: []string{"7", "--body-file", bodyFile}, wantBody: "From a file\n"},
		{name: "-F from stdin", args: []string{"7", "-F", "-"}, stdin: "From stdin\n", wantBody: "From stdin\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			var posted string
			reg.Register(httpmock.REST("POST", commentPath), func(req *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(req.Body)
				posted = string(b)
				return httpmock.StringResponse(http.StatusCreated, created)(req)
			})

			ios, stdin, stdout, _ := iostreams.Test()
			stdin.WriteString(tt.stdin)
			require.NoError(t, runCmd(t, readyFactory(ios), tt.args...))
			want, _ := json.Marshal(map[string]string{"body": tt.wantBody})
			assert.JSONEq(t, string(want), posted)
			assert.Equal(t, commentURL+"\n", stdout.String())
		})
	}
}

func TestNewCmdComment_BodyErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no body flag", args: []string{"7"}, wantErr: "--body, --body-file or --editor required"},
		{name: "both body flags", args: []string{"7", "-b", "x", "-F", "-"}, wantErr: "if any flags in the group [body body-file] are set none of the others can be"},
		{name: "missing body file", args: []string{"7", "-F", filepath.Join(t.TempDir(), "missing.md")}, wantErr: "no such file or directory"},
		{name: "empty body", args: []string{"7", "-b", ""}, wantErr: "body cannot be blank"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			err := runCmd(t, readyFactory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests, "no request without a body")
		})
	}
}

func TestNewCmdComment_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(httpmock.REST("POST", "/api/v1/repos/other/thing/issues/5/comments"),
		httpmock.StringResponse(http.StatusCreated, `{"id":1}`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "https://app.copia.io/other/thing/pulls/5", "-b", "x"))
	assert.Equal(t, "Added comment to pull request other/thing#5\n", stdout.String())
}

func TestNewCmdComment_URLOnOtherHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "https://copia.example.com/other/thing/pulls/5", "-b", "x")
	require.EqualError(t, err, "pull request URL host copia.example.com does not match the active host app.copia.io")
}

func TestNewCmdComment_InvalidArgument(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, readyFactory(ios), "abc", "-b", "x"), "invalid pull request number or URL: abc")
	require.Error(t, runCmd(t, readyFactory(ios)))
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
	require.EqualError(t, runCmd(t, f, "7", "-b", "x"), "no config")
}

func TestNewCmdComment_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "7", "-b", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
