//go:build !windows

package iostreams

import (
	"errors"
	"syscall"
)

// isBrokenPipe reports whether err is a write to a pipe without a reader.
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
