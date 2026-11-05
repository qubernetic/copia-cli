package gitcredential

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain puts go-keyring on its in-memory mock, so no test of this package
// reaches the OS keyring of the machine it runs on.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
