//go:build windows

package iostreams

import (
	"errors"
	"syscall"
)

// isBrokenPipe reports whether err is a write to a pipe without a reader:
// ERROR_BROKEN_PIPE or ERROR_NO_DATA.
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.ERROR_BROKEN_PIPE) || errors.Is(err, syscall.Errno(232))
}
