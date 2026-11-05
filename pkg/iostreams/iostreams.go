// Package iostreams provides TTY-aware I/O abstraction for CLI commands,
// with System() for production and Test() for unit testing.
package iostreams

import (
	"bytes"
	"io"
	"os"
	"os/exec"

	"golang.org/x/term"
)

// defaultTerminalWidth is the width TerminalWidth reports when stdout is no
// terminal or its size cannot be read, as in gh.
const defaultTerminalWidth = 80

// IOStreams provides TTY-aware I/O for commands.
type IOStreams struct {
	In     io.ReadCloser
	Out    io.Writer
	ErrOut io.Writer

	stdinIsTTY  bool
	stdoutIsTTY bool
	stderrIsTTY bool

	pagerCommand string
	pager        *exec.Cmd
	pagerIn      io.WriteCloser
	pagedOut     io.Writer

	// terminalWidth reads the width of the terminal on stdout.
	terminalWidth func() (int, error)
}

// System returns IOStreams connected to real stdin/stdout/stderr.
func System() *IOStreams {
	return &IOStreams{
		In:          os.Stdin,
		Out:         os.Stdout,
		ErrOut:      os.Stderr,
		stdinIsTTY:  isTerminal(os.Stdin),
		stdoutIsTTY: isTerminal(os.Stdout),
		stderrIsTTY: isTerminal(os.Stderr),
		terminalWidth: func() (int, error) {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			return w, err
		},
	}
}

// Test returns IOStreams with in-memory buffers for testing.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return &IOStreams{
		In:     io.NopCloser(stdin),
		Out:    stdout,
		ErrOut: stderr,
	}, stdin, stdout, stderr
}

func (s *IOStreams) IsStdinTTY() bool  { return s.stdinIsTTY }
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutIsTTY }
func (s *IOStreams) IsStderrTTY() bool { return s.stderrIsTTY }

func (s *IOStreams) SetStdinTTY(v bool)  { s.stdinIsTTY = v }
func (s *IOStreams) SetStdoutTTY(v bool) { s.stdoutIsTTY = v }
func (s *IOStreams) SetStderrTTY(v bool) { s.stderrIsTTY = v }

// TerminalWidth returns the width of the terminal on stdout, or 80 when
// stdout is no terminal or its size cannot be read.
func (s *IOStreams) TerminalWidth() int {
	if s.stdoutIsTTY && s.terminalWidth != nil {
		if w, err := s.terminalWidth(); err == nil && w > 0 {
			return w
		}
	}
	return defaultTerminalWidth
}

// SetTerminalWidth sets the width TerminalWidth reports while stdout is a
// terminal.
func (s *IOStreams) SetTerminalWidth(w int) {
	s.terminalWidth = func() (int, error) { return w, nil }
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
