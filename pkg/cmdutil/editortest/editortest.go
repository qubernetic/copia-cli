// Package editortest provides a fake editor command for the tests of the
// commands that take --editor.
package editortest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Editor is a fake editor: a shell script that records its arguments and
// the content of the file it was started on, then replaces that content
// with the text given to New.
type Editor struct {
	// Command is the editor command to set, for example in COPIA_EDITOR.
	Command string
	dir     string
}

// New returns a fake editor that saves text.
func New(t testing.TB, text string) *Editor {
	return newEditor(t, text, "")
}

// NewFailing returns a fake editor that records its file and exits with
// status 3 without saving.
func NewFailing(t testing.TB) *Editor {
	return newEditor(t, "", "exit 3\n")
}

func newEditor(t testing.TB, text, tail string) *Editor {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake editor is a shell script")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "text"), text, 0o600)
	script := "#!/bin/sh\n" +
		"for last; do :; done\n" +
		"printf '%s\\n' \"$@\" > '" + dir + "/args'\n" +
		"cp \"$last\" '" + dir + "/initial'\n" +
		tail +
		"cat '" + dir + "/text' > \"$last\"\n"
	command := filepath.Join(dir, "editor")
	write(t, command, script, 0o700)
	return &Editor{Command: command, dir: dir}
}

// Args returns the arguments the editor was started with, the file last.
func (e *Editor) Args(t testing.TB) []string {
	return strings.Split(strings.TrimSuffix(read(t, filepath.Join(e.dir, "args")), "\n"), "\n")
}

// Path returns the file the editor was started on.
func (e *Editor) Path(t testing.TB) string {
	args := e.Args(t)
	return args[len(args)-1]
}

// Initial returns the content of the file when the editor started.
func (e *Editor) Initial(t testing.TB) string {
	return read(t, filepath.Join(e.dir, "initial"))
}

// Ran reports whether the editor was started.
func (e *Editor) Ran() bool {
	_, err := os.Stat(filepath.Join(e.dir, "args"))
	return err == nil
}

func write(t testing.TB, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("writing the fake editor: %v", err)
	}
}

func read(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the fake editor did not run: %v", err)
	}
	return string(b)
}
