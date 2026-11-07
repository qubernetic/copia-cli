package edit

import (
	"bytes"
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
	tagPath    = "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"
	editPath   = "/api/v1/repos/my-org/my-repo/releases/42"
	releaseURL = "https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0"
)

const releaseJSON = `{"id":42,"tag_name":"v1.0.0","name":"First release","html_url":"` + releaseURL + `"}`

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdEdit(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

// recordBody answers with the edited release and keeps the request body.
func recordBody(body *string) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		*body = string(b)
		return httpmock.StringResponse(http.StatusOK, releaseJSON)(req)
	}
}

func TestNewCmdEdit_Flags(t *testing.T) {
	notesPath := filepath.Join(t.TempDir(), "notes.md")
	require.NoError(t, os.WriteFile(notesPath, []byte("notes from a file\n"), 0o600))

	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantBody string
	}{
		{name: "draft", args: []string{"--draft"}, wantBody: `{"draft":true}`},
		{name: "draft false", args: []string{"--draft=false"}, wantBody: `{"draft":false}`},
		{name: "prerelease", args: []string{"--prerelease"}, wantBody: `{"prerelease":true}`},
		{name: "prerelease false", args: []string{"--prerelease=false"}, wantBody: `{"prerelease":false}`},
		{name: "title", args: []string{"-t", "New title"}, wantBody: `{"name":"New title"}`},
		{name: "long title", args: []string{"--title", "New title"}, wantBody: `{"name":"New title"}`},
		{name: "notes", args: []string{"-n", "New notes"}, wantBody: `{"body":"New notes"}`},
		{name: "notes file", args: []string{"-F", notesPath}, wantBody: `{"body":"notes from a file\n"}`},
		{name: "notes from stdin", args: []string{"--notes-file", "-"}, stdin: "notes from stdin", wantBody: `{"body":"notes from stdin"}`},
		{name: "tag", args: []string{"--tag", "v1.0.1"}, wantBody: `{"tag_name":"v1.0.1"}`},
		{name: "target", args: []string{"--target", "release-1"}, wantBody: `{"target_commitish":"release-1"}`},
		{
			name:     "all",
			args:     []string{"--draft=false", "--prerelease", "-t", "T", "-n", "N", "--tag", "v2", "--target", "main"},
			wantBody: `{"tag_name":"v2","target_commitish":"main","name":"T","body":"N","draft":false,"prerelease":true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			var sent string
			reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
			reg.Register(httpmock.REST("PATCH", editPath), recordBody(&sent))

			ios, stdin, stdout, stderr := iostreams.Test()
			stdin.WriteString(tt.stdin)
			require.NoError(t, runCmd(t, readyFactory(ios), append([]string{"v1.0.0"}, tt.args...)...))
			assert.JSONEq(t, tt.wantBody, sent)
			assert.Equal(t, releaseURL+"\n", stdout.String())
			assert.Empty(t, stderr.String())
			require.Len(t, reg.Requests, 2)
			assert.Equal(t, "token test-token", reg.Requests[1].Header.Get("Authorization"))
			assert.Equal(t, "application/json", reg.Requests[1].Header.Get("Content-Type"))
		})
	}
}

func TestNewCmdEdit_NoFlag(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, stdout, _ := iostreams.Test()
	f := readyFactory(ios)
	f.Repo = ""
	require.EqualError(t, runCmd(t, f, "v1.0.0"), "use flags to specify properties to edit")
	assert.Empty(t, stdout.String())
	assert.Empty(t, reg.Requests)
}

func TestNewCmdEdit_EmptyTitleOrNotes(t *testing.T) {
	emptyPath := filepath.Join(t.TempDir(), "empty.md")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0o600))

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "empty title", args: []string{"--title", ""}, wantErr: "cannot clear a release's title: Copia keeps the old value"},
		{name: "empty title with notes", args: []string{"--title=", "-n", "N"}, wantErr: "cannot clear a release's title: Copia keeps the old value"},
		{name: "empty notes", args: []string{"--notes="}, wantErr: "cannot clear a release's notes: Copia keeps the old value"},
		{name: "empty notes file", args: []string{"-F", emptyPath}, wantErr: "cannot clear a release's notes: Copia keeps the old value"},
		{name: "empty stdin", args: []string{"--notes-file", "-"}, wantErr: "cannot clear a release's notes: Copia keeps the old value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			require.EqualError(t, runCmd(t, readyFactory(ios), append([]string{"v1.0.0"}, tt.args...)...), tt.wantErr)
			assert.Empty(t, stdout.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdEdit_Args(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	require.EqualError(t, runCmd(t, readyFactory(ios), "--draft"), "accepts 1 arg(s), received 0")
}

func TestNewCmdEdit_NotesFileMissing(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "v1.0.0", "-F", filepath.Join(t.TempDir(), "missing.md"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
	assert.Empty(t, reg.Requests)
}

func TestNewCmdEdit_ResolveErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo",
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(t, noConfig, "v1.0.0", "--draft"), "no config")

	noRepo := readyFactory(ios)
	noRepo.Repo = "not-a-repo"
	require.Error(t, runCmd(t, noRepo, "v1.0.0", "--draft"))
}

func TestEditRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name: "release not found",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "release v1.0.0 not found",
		},
		{
			name: "patch refused",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
				reg.Register(httpmock.REST("PATCH", editPath), httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"tag exists"}`))
			},
			wantErr: "failed to edit release (HTTP 422)",
		},
		{
			name: "invalid response",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
				reg.Register(httpmock.REST("PATCH", editPath), httpmock.StringResponse(http.StatusOK, `not json`))
			},
			wantErr: "invalid character 'o' in literal null (expecting 'u')",
		},
		{
			name:    "connection",
			stubs:   func(reg *httpmock.Registry) {},
			wantErr: "no mock matched",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.stubs(reg)
			ios, _, stdout, _ := iostreams.Test()
			draft := true
			err := EditRun(&EditOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Tag:        "v1.0.0",
				Draft:      &draft,
			})
			require.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestEditRun_NoChange(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := EditRun(&EditOptions{IO: ios, Tag: "v1.0.0"})
	require.EqualError(t, err, "use flags to specify properties to edit")
}

func TestEditRun_SendsJSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	var sent string
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"), httpmock.StringResponse(http.StatusOK, releaseJSON))
	reg.Register(httpmock.REST("PATCH", editPath), recordBody(&sent))

	ios, _, stdout, _ := iostreams.Test()
	title := "x"
	require.NoError(t, EditRun(&EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
		Title:      &title,
	}))
	assert.JSONEq(t, `{"name":"x"}`, sent)
	assert.True(t, bytes.HasSuffix(stdout.Bytes(), []byte("\n")))
}
