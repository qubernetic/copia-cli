package cmdutil

import (
	"io"
	"os"
)

// ReadFile reads the named file, or stdin when the name is "-".
// Adapted from github.com/cli/cli/v2/pkg/cmdutil.ReadFile.
func ReadFile(filename string, stdin io.Reader) ([]byte, error) {
	if filename == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(filename)
}
