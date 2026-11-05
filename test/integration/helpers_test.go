//go:build integration

package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	prcreate "github.com/qubernetic/copia-cli/pkg/cmd/pr/create"
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

// apiCall sends one request to the Copia API of env and returns the status
// and the response body. step names the request in failure messages, which
// show the status but never the body.
func apiCall(t *testing.T, env testEnv, step, method, path string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("%s: encode request: %v", step, err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("https://%s/api/v1%s", env.Host, path), body)
	if err != nil {
		t.Fatalf("%s: build request: %v", step, err)
	}
	req.Header.Set("Authorization", "token "+env.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("%s: request failed", step)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s: read response", step)
	}
	return resp.StatusCode, data
}

// prFixture is a pull request between two branches the test created.
type prFixture struct {
	Number int64
	Base   string
	Head   string
}

// createFixturePR creates a base branch from the default branch, a head
// branch from the base with one file commit, and opens a pull request with
// title from head into base. No step writes to the default branch. Cleanup
// closes the pull request and deletes both branches.
func createFixturePR(t *testing.T, env testEnv, title string) prFixture {
	t.Helper()
	fx := createFixtureBranches(t, env)

	status, data := apiCall(t, env, "open pull request", "POST", fmt.Sprintf("/repos/%s/%s/pulls", env.Owner, env.Repo), map[string]string{
		"base":  fx.Base,
		"head":  fx.Head,
		"title": title,
		"body":  "Created by the integration test PR fixture.",
	})
	if status != http.StatusCreated {
		t.Fatalf("open pull request: status %d, want %d", status, http.StatusCreated)
	}
	var pr struct {
		Number int64 `json:"number"`
	}
	if err := json.Unmarshal(data, &pr); err != nil || pr.Number < 1 {
		t.Fatal("open pull request: no number in the response")
	}
	fx.Number = pr.Number
	closeAtCleanup(t, env, fx.Number)
	return fx
}

// createDraftFixturePR is createFixturePR with the pull request opened by
// `pr create --draft`.
func createDraftFixturePR(t *testing.T, env testEnv, title string) prFixture {
	t.Helper()
	fx := createFixtureBranches(t, env)

	ios, out, _ := testIO()
	err := prcreate.CreateRun(&prcreate.CreateOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Title: title,
		Body:  "Created by the integration test PR fixture.",
		Base:  fx.Base,
		Head:  fx.Head,
		Draft: true,
	})
	if err != nil {
		t.Fatal("pr create --draft failed")
	}
	if _, err := fmt.Sscanf(out.String(), "Created PR #%d:", &fx.Number); err != nil || fx.Number < 1 {
		t.Fatal("pr create --draft: no number in the output")
	}
	closeAtCleanup(t, env, fx.Number)
	return fx
}

// createFixtureBranches creates the base and head branches of a fixture pull
// request, the head with one file commit, and deletes both on cleanup.
func createFixtureBranches(t *testing.T, env testEnv) prFixture {
	t.Helper()
	repo := fmt.Sprintf("/repos/%s/%s", env.Owner, env.Repo)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	fx := prFixture{Base: "it-pr-base-" + suffix, Head: "it-pr-head-" + suffix}

	createBranch := func(name, from string) {
		t.Helper()
		payload := map[string]string{"new_branch_name": name}
		if from != "" {
			payload["old_branch_name"] = from
		}
		status, _ := apiCall(t, env, "create branch", "POST", repo+"/branches", payload)
		if status != http.StatusCreated {
			t.Fatalf("create branch: status %d, want %d", status, http.StatusCreated)
		}
		t.Cleanup(func() {
			status, _ := apiCall(t, env, "delete branch", "DELETE", repo+"/branches/"+name, nil)
			if status != http.StatusNoContent {
				t.Errorf("delete branch: status %d, want %d", status, http.StatusNoContent)
			}
		})
	}
	createBranch(fx.Base, "")
	createBranch(fx.Head, fx.Base)

	status, _ := apiCall(t, env, "commit file", "POST", repo+"/contents/integration-test/"+suffix+".txt", map[string]string{
		"branch":  fx.Head,
		"message": "test: add integration fixture file",
		"content": base64.StdEncoding.EncodeToString([]byte("Created by the integration test PR fixture.\n")),
	})
	if status != http.StatusCreated {
		t.Fatalf("commit file: status %d, want %d", status, http.StatusCreated)
	}
	return fx
}

// closeAtCleanup closes pull request number on cleanup. Registered after the
// branches, it runs before they are deleted.
func closeAtCleanup(t *testing.T, env testEnv, number int64) {
	t.Helper()
	t.Cleanup(func() {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d", env.Owner, env.Repo, number)
		status, _ := apiCall(t, env, "close pull request", "PATCH", path, map[string]string{"state": "closed"})
		if status != http.StatusCreated && status != http.StatusOK {
			t.Errorf("close pull request: status %d, want %d", status, http.StatusCreated)
		}
	})
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
