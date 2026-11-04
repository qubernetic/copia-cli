//go:build integration

package integration

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type testEnv struct {
	Host  string
	Token string
	Owner string
	Repo  string
}

func loadTestEnv(t *testing.T) testEnv {
	t.Helper()

	host := os.Getenv("COPIA_TEST_HOST")
	token := os.Getenv("COPIA_TEST_TOKEN")
	owner := os.Getenv("COPIA_TEST_OWNER")
	repo := os.Getenv("COPIA_TEST_REPO")

	if host == "" || token == "" || owner == "" || repo == "" {
		t.Skip("Integration test env vars not set (COPIA_TEST_HOST, COPIA_TEST_TOKEN, COPIA_TEST_OWNER, COPIA_TEST_REPO)")
	}

	return testEnv{
		Host:  host,
		Token: token,
		Owner: owner,
		Repo:  repo,
	}
}

// testIO returns IOStreams and stdout buffer for asserting CLI output.
func testIO() (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return ios, stdout, stderr
}

// testHTTPClient returns a real HTTP client for integration tests.
func testHTTPClient() *http.Client {
	return &http.Client{}
}

// The assertions below check command output and response bodies, which hold
// the test account's data. The CI log of this public repository shows a
// failing test's messages, so they name the expected text and the length of
// the output in bytes, never the output itself. test/workflows keeps every
// other file of this package from logging or asserting on output directly.

// assertOutputContains fails the test when out does not contain want.
func assertOutputContains(t *testing.T, out, want string) bool {
	t.Helper()
	if strings.Contains(out, want) {
		return true
	}
	t.Errorf("output (%d bytes) does not contain %q", len(out), want)
	return false
}

// assertOutputEqual fails the test when out is not want.
func assertOutputEqual(t *testing.T, out, want string) bool {
	t.Helper()
	if out == want {
		return true
	}
	t.Errorf("output (%d bytes) is not %q (%d bytes)", len(out), want, len(want))
	return false
}

// assertOutputEmpty fails the test when out is not empty.
func assertOutputEmpty(t *testing.T, out string) bool {
	t.Helper()
	if out == "" {
		return true
	}
	t.Errorf("output (%d bytes) is not empty", len(out))
	return false
}

// assertOutputNotEmpty fails the test when out is empty.
func assertOutputNotEmpty(t *testing.T, out string) bool {
	t.Helper()
	if out != "" {
		return true
	}
	t.Error("output is empty")
	return false
}

// assertStatusCode fails the test when the response status is not want.
func assertStatusCode(t *testing.T, got, want int) bool {
	t.Helper()
	if got == want {
		return true
	}
	t.Errorf("status %d, want %d", got, want)
	return false
}
