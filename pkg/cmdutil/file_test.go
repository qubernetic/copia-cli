package cmdutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(path, []byte("from file"), 0o600))

	got, err := ReadFile(path, strings.NewReader("unused"))
	require.NoError(t, err)
	assert.Equal(t, "from file", string(got))

	got, err = ReadFile("-", strings.NewReader("from stdin"))
	require.NoError(t, err)
	assert.Equal(t, "from stdin", string(got))

	_, err = ReadFile(filepath.Join(t.TempDir(), "missing"), nil)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
