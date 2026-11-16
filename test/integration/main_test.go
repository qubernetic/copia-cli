//go:build integration

package integration

import (
	"fmt"
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
	code := m.Run()
	// The scratch repositories, then the run repository, go with everything
	// the tests left in them. The message names neither the owner nor a
	// repository.
	if runRepo.name != "" {
		api := newAPI(os.Getenv("COPIA_TEST_HOST"), os.Getenv("COPIA_TEST_TOKEN"))
		if err := tearDownRun(api, os.Getenv("COPIA_TEST_OWNER"), runRepo.name, runRepo.scratch); err != nil {
			fmt.Fprintf(os.Stderr, "tear down the run repositories: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}
