//go:build integration

package integration

import (
	"os"
	"testing"

	"github.com/qubernetic/copia-cli/internal/copiacmd"
)

// runCLIEnv set to 1 makes the test binary run copia-cli with its arguments
// instead of the tests, so a test can hand it to git as a credential helper.
const runCLIEnv = "COPIA_INTEGRATION_RUN_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(runCLIEnv) == "1" {
		os.Exit(copiacmd.Main())
	}
	os.Exit(m.Run())
}
