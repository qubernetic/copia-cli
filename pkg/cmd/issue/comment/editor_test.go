package comment

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

func TestNewCmdComment_Editor(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var posted string
	reg.Register(httpmock.REST("POST", commentPath), func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		posted = string(b)
		return httpmock.StringResponse(http.StatusCreated, `{"id":1}`)(req)
	})

	editor := editortest.New(t, "\nWritten in the editor\n")
	t.Setenv("COPIA_EDITOR", editor.Command)
	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "12", "--editor"))

	assert.JSONEq(t, `{"body":"Written in the editor"}`, posted)
	assert.Equal(t, "Comment added to issue #12\n", stdout.String())
	assert.Equal(t, "", editor.Initial(t))
	assert.NoFileExists(t, editor.Path(t))
}

func TestNewCmdComment_EditorErrors(t *testing.T) {
	tests := []struct {
		name    string
		saved   string
		noEdit  bool
		args    []string
		wantErr string
		wantRan bool
	}{
		{name: "empty text", saved: " \n", args: []string{"12", "-e"}, wantErr: "aborted: the editor text is empty", wantRan: true},
		{name: "no editor", noEdit: true, args: []string{"12", "-e"}, wantErr: "no editor set: set COPIA_EDITOR, the config key editor (copia-cli config set editor <command>), VISUAL or EDITOR"},
		{name: "with --body", saved: "x", args: []string{"12", "-e", "--body", "x"}, wantErr: "if any flags in the group [body editor] are set none of the others can be; [body editor] were all set"},
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
