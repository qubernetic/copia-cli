package edit

import (
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

func TestNewCmdEdit_Flags(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(bodyFile, []byte("From a file\n"), 0o600))

	tests := []struct {
		name  string
		args  []string
		stdin string
		wantJ string
	}{
		{name: "--title", args: []string{"7", "--title", "T"}, wantJ: `{"title":"T"}`},
		{name: "-t", args: []string{"7", "-t", "T"}, wantJ: `{"title":"T"}`},
		{name: "-b", args: []string{"7", "-b", "B"}, wantJ: `{"body":"B"}`},
		{name: "empty --body clears it", args: []string{"7", "--body", ""}, wantJ: `{"body":""}`},
		{name: "--body-file", args: []string{"7", "--body-file", bodyFile}, wantJ: `{"body":"From a file\n"}`},
		{name: "-F from stdin", args: []string{"7", "-F", "-"}, stdin: "From stdin\n", wantJ: `{"body":"From stdin\n"}`},
		{name: "-B", args: []string{"7", "-B", "develop"}, wantJ: `{"base":"develop"}`},
		{name: "hash number", args: []string{"#7", "--base", "develop"}, wantJ: `{"base":"develop"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			patched := recordPatch(reg, prPath, http.StatusCreated, updated)

			ios, stdin, stdout, _ := iostreams.Test()
			stdin.WriteString(tt.stdin)
			require.NoError(t, runCmd(t, readyFactory(ios), tt.args...))
			assert.JSONEq(t, tt.wantJ, *patched)
			assert.Equal(t, prURL+"\n", stdout.String())
		})
	}
}

func TestNewCmdEdit_ListFlags(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	stubLabels(reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/v1.0"),
		httpmock.StringResponse(http.StatusOK, `{"id":5}`))
	reg.Register(httpmock.REST("GET", prPath), httpmock.StringResponse(http.StatusOK, current))
	patched := recordPatch(reg, prPath, http.StatusCreated, updated)
	reg.Register(httpmock.REST("POST", prPath+"/requested_reviewers"), httpmock.StringResponse(http.StatusCreated, `[]`))
	reg.Register(httpmock.REST("DELETE", prPath+"/requested_reviewers"), httpmock.StringResponse(http.StatusNoContent, ``))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "7",
		"--add-label", "urgent", "--remove-label", "bug",
		"--add-assignee", "jane", "--remove-assignee", "john",
		"--add-reviewer", "jane", "--remove-reviewer", "john",
		"-m", "v1.0"))
	assert.JSONEq(t, `{"labels":[2,3],"assignees":["jane"],"milestone":5}`, *patched)
	assert.Equal(t, prURL+"\n", stdout.String())
}

func TestNewCmdEdit_RemoveMilestone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	patched := recordPatch(reg, "/api/v1/repos/my-org/my-repo/issues/7", http.StatusCreated, `{}`)

	ios, _, _, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "7", "--remove-milestone"))
	assert.JSONEq(t, `{"milestone":0}`, *patched)
}

func TestNewCmdEdit_FailsBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no flag", args: []string{"7"}, wantErr: "nothing to edit"},
		{name: "body and body-file", args: []string{"7", "-b", "x", "-F", "-"}, wantErr: "if any flags in the group [body body-file] are set none of the others can be"},
		{name: "milestone and remove-milestone", args: []string{"7", "-m", "v1.0", "--remove-milestone"}, wantErr: "specify only one of --milestone or --remove-milestone"},
		{name: "same label", args: []string{"7", "--add-label", "bug", "--remove-label", "bug"}, wantErr: `label "bug" cannot be both added and removed`},
		{name: "same assignee", args: []string{"7", "--add-assignee", "a,b", "--remove-assignee", "b"}, wantErr: `assignee "b" cannot be both added and removed`},
		{name: "same reviewer", args: []string{"7", "--add-reviewer", "o/t", "--remove-reviewer", "o/t"}, wantErr: `reviewer "o/t" cannot be both added and removed`},
		{name: "empty title", args: []string{"7", "--title", ""}, wantErr: "title cannot be blank"},
		{name: "missing body file", args: []string{"7", "-F", filepath.Join(t.TempDir(), "missing.md")}, wantErr: "no such file or directory"},
		{name: "invalid argument", args: []string{"abc", "-t", "x"}, wantErr: "invalid pull request number or URL: abc"},
		{name: "URL on another host", args: []string{"https://copia.example.com/o/r/pulls/5", "-t", "x"}, wantErr: "pull request URL host copia.example.com does not match the active host app.copia.io"},
		{name: "no argument", args: []string{}, wantErr: "accepts 1 arg(s), received 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			err := runCmd(t, readyFactory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdEdit_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	patched := recordPatch(reg, "/api/v1/repos/other/thing/pulls/5", http.StatusCreated, `{}`)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "https://app.copia.io/other/thing/pulls/5", "-t", "T"))
	assert.JSONEq(t, `{"title":"T"}`, *patched)
	assert.Equal(t, "Updated pull request other/thing#5\n", stdout.String())
}

func TestNewCmdEdit_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Repo:      "my-org/my-repo",
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	require.EqualError(t, runCmd(t, f, "7", "-t", "x"), "no config")
}

func TestNewCmdEdit_RepoFailure(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runCmd(t, f, "7", "-t", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")
}
