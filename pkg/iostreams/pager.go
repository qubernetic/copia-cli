package iostreams

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// SetPager sets the pager command that StartPager runs.
func (s *IOStreams) SetPager(command string) { s.pagerCommand = command }

// GetPager returns the pager command that StartPager runs.
func (s *IOStreams) GetPager() string { return s.pagerCommand }

// StartPager sends Out through the pager command until StopPager, when
// stdout is a TTY and a pager other than cat is set. The command is split
// into words at whitespace and run without a shell. When the pager cannot
// be started, Out stays unchanged and the error is returned.
func (s *IOStreams) StartPager() error {
	words := strings.Fields(s.pagerCommand)
	if len(words) == 0 || words[0] == "cat" || !s.stdoutIsTTY || s.pager != nil {
		return nil
	}

	cmd := exec.Command(words[0], words[1:]...)
	cmd.Env = pagerEnv(os.Environ())
	cmd.Stdout = s.Out
	cmd.Stderr = s.ErrOut
	in, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("starting pager %q: %w", words[0], err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting pager %q: %w", words[0], err)
	}

	s.pager = cmd
	s.pagerIn = in
	s.pagedOut = s.Out
	s.Out = &pagerWriter{w: in}
	return nil
}

// StopPager closes the pager's input, waits for the pager to exit and
// restores Out. It does nothing when no pager runs.
func (s *IOStreams) StopPager() {
	if s.pager == nil {
		return
	}
	_ = s.pagerIn.Close()
	_ = s.pager.Wait()
	s.Out = s.pagedOut
	s.pager, s.pagerIn, s.pagedOut = nil, nil, nil
}

// pagerEnv returns env with the defaults of less and lv that gh sets,
// unless the user set them.
func pagerEnv(env []string) []string {
	if _, ok := os.LookupEnv("LESS"); !ok {
		env = append(env, "LESS=FRX")
	}
	if _, ok := os.LookupEnv("LV"); !ok {
		env = append(env, "LV=-c")
	}
	return env
}

// pagerWriter writes to the pager's input. Once the pager has exited (the
// user quit it before reading everything), writes are discarded instead of
// failing with a broken pipe.
type pagerWriter struct {
	w      io.Writer
	closed bool
}

func (p *pagerWriter) Write(b []byte) (int, error) {
	if p.closed {
		return len(b), nil
	}
	n, err := p.w.Write(b)
	if err != nil && (errors.Is(err, io.ErrClosedPipe) || isBrokenPipe(err)) {
		p.closed = true
		return len(b), nil
	}
	return n, err
}
