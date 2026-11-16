package create

import (
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil/editortest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate_Editor(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		saved       string
		wantInitial string
		wantSent    string
	}{
		{
			name:     "title and body from the editor",
			args:     []string{"--editor"},
			saved:    "Fix sensor mapping\n\nThe mapping of input 3 is off.\n",
			wantSent: `{"title":"Fix sensor mapping","body":"The mapping of input 3 is off."}`,
		},
		{
			name:        "--title prefills the first line",
			args:        []string{"-e", "--title", "Draft title"},
			saved:       "Final title\n",
			wantInitial: "Draft title\n\n",
			wantSent:    `{"title":"Final title"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			var sent string
			reg.Register(httpmock.REST("POST", issuesPath), func(req *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(req.Body)
				sent = string(b)
				return httpmock.StringResponse(http.StatusCreated, `{"number":5,"title":"T","html_url":"https://app.copia.io/my-org/my-repo/issues/5"}`)(req)
			})

			editor := editortest.New(t, tt.saved)
			t.Setenv("COPIA_EDITOR", editor.Command)
			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runCmd(t, readyFactory(ios), tt.args...))

			assert.JSONEq(t, tt.wantSent, sent)
			assert.Equal(t, tt.wantInitial, editor.Initial(t))
			assert.Contains(t, stdout.String(), "Created issue #5")
			assert.NoFileExists(t, editor.Path(t))
		})
	}
}

func TestNewCmdCreate_EditorErrors(t *testing.T) {
	tests := []struct {
		name    string
		saved   string
		noEdit  bool
		args    []string
		wantErr string
		wantRan bool
	}{
		{name: "empty text", saved: "\n\n", args: []string{"-e", "-t", "Draft"}, wantErr: "aborted: the editor text is empty", wantRan: true},
		{name: "blank title line", saved: "   \n", args: []string{"-e"}, wantErr: "aborted: the editor text is empty", wantRan: true},
		{name: "no editor", noEdit: true, args: []string{"--editor"}, wantErr: "no editor set: set COPIA_EDITOR, the config key editor (copia-cli config set editor <command>), VISUAL or EDITOR"},
		{name: "with --body", saved: "T", args: []string{"-e", "--body", "x"}, wantErr: "if any flags in the group [body editor] are set none of the others can be; [body editor] were all set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)
			editor := editortest.New(t, tt.saved)
			t.Setenv("COPIA_EDITOR", editor.Command)
			if tt.noEdit {
				t.Setenv("COPIA_EDITOR", "")
				t.Setenv("VISUAL", "")
				t.Setenv("EDITOR", "")
			}
			ios, _, stdout, _ := iostreams.Test()
			err := runCmd(t, readyFactory(ios), tt.args...)
			require.EqualError(t, err, tt.wantErr)
			assert.Equal(t, tt.wantRan, editor.Ran())
			assert.Empty(t, reg.Requests)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestCreateRun_EditorRunsBeforeLookups(t *testing.T) {
	reg := &httpmock.Registry{}
	editor := editortest.New(t, "")
	opts, stdout := runOpts(reg)
	opts.Labels = []string{"bug"}
	opts.EditorMode = true
	opts.EditorCommand = editor.Command

	require.EqualError(t, CreateRun(opts), "aborted: the editor text is empty")
	assert.Empty(t, reg.Requests, "no label lookup before the editor")
	assert.Empty(t, stdout.String())
}
