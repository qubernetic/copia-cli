package cmdutil

import (
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/editortest"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactory_Editor(t *testing.T) {
	tests := []struct {
		name   string
		env    string
		config string
		visual string
		editor string
		want   string
	}{
		{name: "none"},
		{name: "EDITOR", editor: "nano", want: "nano"},
		{name: "VISUAL beats EDITOR", visual: "code --wait", editor: "nano", want: "code --wait"},
		{name: "config beats VISUAL", config: "vim", visual: "code --wait", editor: "nano", want: "vim"},
		{name: "env beats config", env: "emacs", config: "vim", visual: "code", editor: "nano", want: "emacs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_EDITOR", tt.env)
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)
			f := &Factory{Config: func() (*config.Config, error) { return &config.Config{Editor: tt.config}, nil }}
			assert.Equal(t, tt.want, f.Editor())
		})
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr string
	}{
		{in: "", want: nil},
		{in: "  ", want: nil},
		{in: "vim", want: []string{"vim"}},
		{in: " code  --wait\t-n ", want: []string{"code", "--wait", "-n"}},
		{in: `"/Applications/Sublime Text.app/subl" -w`, want: []string{"/Applications/Sublime Text.app/subl", "-w"}},
		{in: `'my editor' '$HOME'`, want: []string{"my editor", "$HOME"}},
		{in: `my\ editor a\\b`, want: []string{"my editor", `a\b`}},
		{in: `"a \"b\" \$c \x"`, want: []string{`a "b" $c \x`}},
		{in: `a""b ''`, want: []string{"ab", ""}},
		{in: `vim; rm -rf x`, want: []string{"vim;", "rm", "-rf", "x"}},
		{in: `"vim`, wantErr: "unterminated double quote"},
		{in: `'vim`, wantErr: "unterminated single quote"},
		{in: `vim\`, wantErr: "unterminated escape"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := SplitCommand(tt.in)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEditText(t *testing.T) {
	editor := editortest.New(t, "saved text\n")
	ios, _, _, _ := iostreams.Test()

	got, err := EditText(ios, editor.Command+" --wait", "initial\n")
	require.NoError(t, err)
	assert.Equal(t, "saved text\n", got)
	assert.Equal(t, "initial\n", editor.Initial(t))

	args := editor.Args(t)
	require.Len(t, args, 2)
	assert.Equal(t, "--wait", args[0])
	assert.Regexp(t, `copia-.*\.md$`, args[1])
	assert.NoFileExists(t, editor.Path(t))
}

func TestEditText_Errors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	_, err := EditText(ios, "  ", "")
	require.EqualError(t, err, "no editor set: set COPIA_EDITOR, the config key editor (copia-cli config set editor <command>), VISUAL or EDITOR")

	_, err = EditText(ios, `"vim`, "")
	require.EqualError(t, err, `invalid editor command "\"vim": unterminated double quote`)

	_, err = EditText(ios, "copia-no-such-editor", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `running the editor "copia-no-such-editor"`)

	failing := editortest.NewFailing(t)
	_, err = EditText(ios, failing.Command, "draft")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "running the editor")
	assert.Contains(t, err.Error(), "exit status 3")
	assert.NoFileExists(t, failing.Path(t))
}

func TestEditTitleBody(t *testing.T) {
	tests := []struct {
		name        string
		title, body string
		saved       string
		wantInitial string
		wantTitle   string
		wantBody    string
		wantErr     string
	}{
		{name: "title and body", saved: "Fix it\n\nThe body\n  indented\n", wantInitial: "", wantTitle: "Fix it", wantBody: "The body\n  indented"},
		{name: "prefilled", title: "T", body: "B", saved: "T2\nB2", wantInitial: "T\n\nB", wantTitle: "T2", wantBody: "B2"},
		{name: "title only", title: "T", saved: "  Only title  \n", wantInitial: "T\n\n", wantTitle: "Only title"},
		{name: "CRLF and leading blank lines", saved: "\r\n\r\nTitle\r\n\r\nLine 1\r\nLine 2\r\n", wantTitle: "Title", wantBody: "Line 1\nLine 2"},
		{name: "empty", title: "T", saved: " \n\t\n", wantInitial: "T\n\n", wantErr: "aborted: the editor text is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := editortest.New(t, tt.saved)
			ios, _, _, _ := iostreams.Test()
			title, body, err := EditTitleBody(ios, editor.Command, tt.title, tt.body)
			assert.Equal(t, tt.wantInitial, editor.Initial(t))
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTitle, title)
			assert.Equal(t, tt.wantBody, body)
		})
	}
}

func TestEditTitleBody_EditorFails(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	_, _, err := EditTitleBody(ios, "", "T", "")
	require.ErrorContains(t, err, "no editor set")
}

func TestEditBody(t *testing.T) {
	tests := []struct {
		name    string
		saved   string
		want    string
		wantErr string
	}{
		{name: "text", saved: "\n\n  code\nmore\n\n", want: "  code\nmore"},
		{name: "CRLF", saved: "a\r\nb\r\n", want: "a\nb"},
		{name: "empty", saved: "", wantErr: "aborted: the editor text is empty"},
		{name: "blank", saved: " \r\n\t\n", wantErr: "aborted: the editor text is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := editortest.New(t, tt.saved)
			ios, _, _, _ := iostreams.Test()
			got, err := EditBody(ios, editor.Command)
			assert.Equal(t, "", editor.Initial(t))
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEditBody_EditorFails(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	_, err := EditBody(ios, "")
	require.ErrorContains(t, err, "no editor set")
}
