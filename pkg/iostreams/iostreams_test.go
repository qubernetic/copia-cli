package iostreams

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTest_ReturnsWorkingStreams(t *testing.T) {
	ios, stdin, stdout, stderr := Test()

	stdin.WriteString("input data")
	assert.NotNil(t, ios)
	assert.Equal(t, "input data", stdin.String())
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())
	assert.False(t, ios.IsStdoutTTY())
	assert.False(t, ios.IsStdinTTY())
}

func TestIOStreams_SetAndGetTTY(t *testing.T) {
	ios, _, _, _ := Test()

	ios.SetStdoutTTY(true)
	assert.True(t, ios.IsStdoutTTY())

	ios.SetStdoutTTY(false)
	assert.False(t, ios.IsStdoutTTY())

	ios.SetStdinTTY(true)
	assert.True(t, ios.IsStdinTTY())

	ios.SetStderrTTY(true)
	assert.True(t, ios.IsStderrTTY())
}

func TestTest_WriteToOut(t *testing.T) {
	ios, _, stdout, _ := Test()

	_, err := ios.Out.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, "hello", stdout.String())
}

func TestTest_WriteToErrOut(t *testing.T) {
	ios, _, _, stderr := Test()

	_, err := ios.ErrOut.Write([]byte("error msg"))
	assert.NoError(t, err)
	assert.Equal(t, "error msg", stderr.String())
}

func TestIOStreams_TerminalWidth(t *testing.T) {
	tests := []struct {
		name  string
		tty   bool
		width func() (int, error)
		want  int
	}{
		{name: "not a terminal", tty: false, width: func() (int, error) { return 120, nil }, want: 80},
		{name: "terminal", tty: true, width: func() (int, error) { return 120, nil }, want: 120},
		{name: "size unknown", tty: true, width: func() (int, error) { return 0, errors.New("no size") }, want: 80},
		{name: "zero width", tty: true, width: func() (int, error) { return 0, nil }, want: 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := Test()
			ios.SetStdoutTTY(tt.tty)
			ios.terminalWidth = tt.width
			assert.Equal(t, tt.want, ios.TerminalWidth())
		})
	}
}

func TestIOStreams_SetTerminalWidth(t *testing.T) {
	ios, _, _, _ := Test()
	assert.Equal(t, 80, ios.TerminalWidth(), "Test streams are no terminal")

	ios.SetStdoutTTY(true)
	ios.SetTerminalWidth(100)
	assert.Equal(t, 100, ios.TerminalWidth())
}

func TestSystem_TerminalWidthReadsStdout(t *testing.T) {
	// Under go test stdout is a pipe or file, so the size cannot be read.
	_, err := System().terminalWidth()
	assert.Error(t, err)
}
