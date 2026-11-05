package create

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/editortest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate_Editor(t *testing.T) {
	tests := []struct {
		name        string
		messages    []string
		args        []string
		saved       string
		wantInitial string
		wantTitle   string
		wantBody    string
	}{
		{
			name:      "title and body from the editor",
			args:      []string{"--editor", "--head", "feature/x"},
			saved:     "feat: add wrapper\n\nWraps the PLC tags.\n",
			wantTitle: "feat: add wrapper",
			wantBody:  "Wraps the PLC tags.",
		},
		{
			name:        "--title prefills, --draft applies to the saved title",
			args:        []string{"-e", "-d", "-t", "feat: draft", "--head", "feature/x"},
			saved:       "feat: final\n",
			wantInitial: "feat: draft\n\n",
			wantTitle:   "WIP: feat: final",
		},
		{
			name:        "--fill prefills title and body",
			messages:    []string{"feat: one\n\nFirst line."},
			args:        []string{"-e", "--fill", "--base", "main"},
			saved:       "feat: one, edited\n\nFirst line.\nSecond line.\n",
			wantInitial: "feat: one\n\nFirst line.",
			wantTitle:   "feat: one, edited",
			wantBody:    "First line.\nSecond line.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.messages != nil {
				gitRepo(t, tt.messages...)
			}
			editor := editortest.New(t, tt.saved)
			t.Setenv("COPIA_EDITOR", editor.Command)
			ios, _, _, _ := iostreams.Test()
			sent := runCreateSent(t, ios, tt.args...)

			assert.Equal(t, tt.wantTitle, sent["title"])
			assert.Equal(t, tt.wantBody, sent["body"])
			assert.Equal(t, tt.wantInitial, editor.Initial(t))
			assert.NoFileExists(t, editor.Path(t))
		})
	}
}

func TestCreate_EditorErrors(t *testing.T) {
	tests := []struct {
		name    string
		saved   string
		noEdit  bool
		args    []string
		wantErr string
		wantRan bool
	}{
		{name: "empty text", args: []string{"-e", "-t", "feat: x"}, wantErr: "aborted: the editor text is empty", wantRan: true},
		{name: "empty text with metadata", args: []string{"-e", "--label", "bug", "--reviewer", "jane"}, wantErr: "aborted: the editor text is empty", wantRan: true},
		{name: "no editor", noEdit: true, args: []string{"--editor"}, wantErr: "no editor set: set COPIA_EDITOR, the config key editor (copia-cli config set editor <command>), VISUAL or EDITOR"},
		{name: "with --body", saved: "x", args: []string{"-e", "-b", "x"}, wantErr: "if any flags in the group [body editor] are set none of the others can be; [body editor] were all set"},
		{name: "with --body-file", saved: "x", args: []string{"-e", "-F", "-"}, wantErr: "if any flags in the group [body-file editor] are set none of the others can be; [body-file editor] were all set"},
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
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			err := runCreate(t, f, append([]string{"--head", "feature/x"}, tt.args...)...)
			require.EqualError(t, err, tt.wantErr)
			assert.Equal(t, tt.wantRan, editor.Ran())
			assert.Empty(t, reg.Requests)
			assert.Empty(t, stdout.String())
		})
	}
}
