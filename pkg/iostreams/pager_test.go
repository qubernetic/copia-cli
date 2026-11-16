package iostreams

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePager returns a pager command that runs this test binary as
// TestHelperPager in the given mode.
func fakePager(t *testing.T, mode string) string {
	t.Helper()
	t.Setenv("COPIA_WANT_HELPER_PAGER", "1")
	return os.Args[0] + " -test.run=^TestHelperPager$ -- " + mode
}

// TestHelperPager is the fake pager: in mode "copy" it writes its words, the
// LESS variable and its input, in mode "exit" it exits without reading.
func TestHelperPager(t *testing.T) {
	if os.Getenv("COPIA_WANT_HELPER_PAGER") != "1" {
		t.Skip("helper process")
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) > 1 && args[1] == "copy" {
		input, _ := io.ReadAll(os.Stdin)
		fmt.Printf("pager %s LESS=%s\n%s", strings.Join(args[1:], " "), os.Getenv("LESS"), input)
	}
	os.Exit(0)
}

func TestStartPager_PagesTTYOutput(t *testing.T) {
	ios, _, stdout, _ := Test()
	ios.SetStdoutTTY(true)
	ios.SetPager(fakePager(t, "copy word"))

	require.NoError(t, ios.StartPager())
	_, err := fmt.Fprint(ios.Out, "hello\n")
	require.NoError(t, err)
	ios.StopPager()

	assert.Equal(t, "pager copy word LESS=FRX\nhello\n", stdout.String())
}

func TestStartPager_KeepsLESS(t *testing.T) {
	t.Setenv("LESS", "R")
	ios, _, stdout, _ := Test()
	ios.SetStdoutTTY(true)
	ios.SetPager(fakePager(t, "copy"))

	require.NoError(t, ios.StartPager())
	ios.StopPager()

	assert.Equal(t, "pager copy LESS=R\n", stdout.String())
}

func TestStartPager_NoPager(t *testing.T) {
	tests := []struct {
		name  string
		tty   bool
		pager string
	}{
		{name: "stdout is not a TTY", tty: false, pager: "less"},
		{name: "no pager set", tty: true, pager: ""},
		{name: "blank pager", tty: true, pager: "  "},
		{name: "cat", tty: true, pager: "cat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetPager(tt.pager)

			require.NoError(t, ios.StartPager())
			_, err := fmt.Fprint(ios.Out, "hello\n")
			require.NoError(t, err)
			ios.StopPager()

			assert.Equal(t, "hello\n", stdout.String())
		})
	}
}

func TestStartPager_EarlyExitIsNotAnError(t *testing.T) {
	ios, _, stdout, _ := Test()
	ios.SetStdoutTTY(true)
	ios.SetPager(fakePager(t, "exit"))

	require.NoError(t, ios.StartPager())
	chunk := bytes.Repeat([]byte("x"), 64*1024)
	for range 64 {
		n, err := ios.Out.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	ios.StopPager()

	assert.Empty(t, stdout.String())
}

func TestStartPager_UnknownCommand(t *testing.T) {
	ios, _, stdout, _ := Test()
	ios.SetStdoutTTY(true)
	ios.SetPager("copia-no-such-pager -R")

	assert.Error(t, ios.StartPager())
	_, err := fmt.Fprint(ios.Out, "hello\n")
	require.NoError(t, err)
	ios.StopPager()

	assert.Equal(t, "hello\n", stdout.String())
}
