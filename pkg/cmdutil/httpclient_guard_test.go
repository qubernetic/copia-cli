package cmdutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoUnguardedHTTPClients fails when production code under pkg/ or
// internal/ builds an HTTP client without the same-origin redirect policy of
// NewHTTPClient.
func TestNoUnguardedHTTPClients(t *testing.T) {
	forbidden := []string{"&http.Client{}", "http.DefaultClient"}
	root := filepath.Join("..", "..")
	for _, dir := range []string{"pkg", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, f := range forbidden {
				if strings.Contains(string(src), f) {
					t.Errorf("%s contains %s; use cmdutil.NewHTTPClient()", path, f)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
