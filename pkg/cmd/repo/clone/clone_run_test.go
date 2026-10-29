package clone

import (
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGit runs git with the current (test) environment and fails on error.
func runGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// isolateGit keeps the user's and the system's git config out of the test.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

// newBareRepo creates a bare repo at <parent>/my-org/my-repo.git with one
// commit and returns parent.
func newBareRepo(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	src := t.TempDir()
	runGit(t, "init", "-q", src)
	runGit(t, "-C", src, "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, "clone", "-q", "--bare", src, filepath.Join(parent, "my-org", "my-repo.git"))
	return parent
}

// setupRemote creates a bare repo with newBareRepo and rewrites the Copia
// URLs CloneRun builds to point at it via git's environment config. It then
// chdirs into an empty work dir.
func setupRemote(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	parent := newBareRepo(t)

	base := "url.file://" + filepath.ToSlash(parent) + "/.insteadOf"
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", base)
	t.Setenv("GIT_CONFIG_VALUE_0", "https://app.copia.io/")
	t.Setenv("GIT_CONFIG_KEY_1", base)
	t.Setenv("GIT_CONFIG_VALUE_1", "git@app.copia.io:")

	work := t.TempDir()
	t.Chdir(work)
	return work
}

// storedOrigin returns the raw origin URL from the clone's config. Unlike
// `git remote get-url`, it is not rewritten by the insteadOf rules above.
func storedOrigin(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, "-C", dir, "config", "--get", "remote.origin.url")
}

func newCloneCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdClone(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdClone_Run(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantDir string
	}{
		{name: "explicit directory", args: []string{"my-org/my-repo", "target"}, wantDir: "target"},
		{name: "derived directory", args: []string{"my-org/my-repo"}, wantDir: "my-repo"},
		{name: "https URL", args: []string{"https://app.copia.io/my-org/my-repo.git"}, wantDir: "my-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work := setupRemote(t)
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

			err := newCloneCmd(f, tt.args...)
			require.NoError(t, err)

			dir := filepath.Join(work, tt.wantDir)
			assert.DirExists(t, filepath.Join(dir, ".git"))
			assert.Equal(t, "init", runGit(t, "-C", dir, "log", "-1", "--format=%s"))

			origin := storedOrigin(t, dir)
			assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", origin)
			assert.NotContains(t, origin, "test-token")
		})
	}
}

func TestNewCmdClone_CloneFailure(t *testing.T) {
	work := setupRemote(t)
	ios, _, _, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	err := newCloneCmd(f, "my-org/missing")
	require.Error(t, err)
	assert.NotEmpty(t, stderr.String())
	assert.NoDirExists(t, filepath.Join(work, "missing"))
}

func TestNewCmdClone_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}

	err := newCloneCmd(f, "my-org/my-repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}

func TestNewCmdClone_Args(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	assert.Error(t, newCloneCmd(f))
	assert.Error(t, newCloneCmd(f, "a/b", "dir", "extra"))
}

func TestCloneRun_NoToken(t *testing.T) {
	work := setupRemote(t)
	ios, _, _, _ := iostreams.Test()

	err := CloneRun(&CloneOptions{IO: ios, Host: "app.copia.io", Repo: "my-org/my-repo"})
	require.NoError(t, err)
	assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", storedOrigin(t, filepath.Join(work, "my-repo")))
}

func TestCloneRun_InvalidConfigCount(t *testing.T) {
	work := setupRemote(t)
	t.Setenv("GIT_CONFIG_COUNT", "bogus")
	ios, _, _, _ := iostreams.Test()

	err := CloneRun(&CloneOptions{IO: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `GIT_CONFIG_COUNT "bogus"`)
	assert.NoDirExists(t, filepath.Join(work, "my-repo"))
}

func TestCloneRun_SSHKeepsURL(t *testing.T) {
	work := setupRemote(t)
	ios, _, _, _ := iostreams.Test()

	err := CloneRun(&CloneOptions{
		IO:    ios,
		Host:  "app.copia.io",
		Token: "test-token",
		Repo:  "git@app.copia.io:my-org/my-repo.git",
		Dir:   "ssh-clone",
	})
	require.NoError(t, err)
	assert.Equal(t, "git@app.copia.io:my-org/my-repo.git", storedOrigin(t, filepath.Join(work, "ssh-clone")))
}


// TestCloneCommand_GitScopesCredential checks with the real git that the
// helper cloneCommand configures answers for the configured host only,
// replaces the user's helpers there and nowhere else, and stores nothing.
func TestCloneCommand_GitScopesCredential(t *testing.T) {
	isolateGit(t)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "!f() { echo username=user; echo password=user-secret; }; f")

	_, env := cloneCommand(&CloneOptions{Host: "app.copia.io", Token: "test-token", Repo: "o/r"}, 1)

	credential := func(action, input string) string {
		cmd := exec.Command("git", "credential", action)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdin = strings.NewReader(input)
		out, _ := cmd.Output()
		return string(out)
	}
	fill := func(target string) string { return credential("fill", "url="+target+"\n\n") }

	for _, target := range []string{
		"https://app.copia.io/o/r.git",
		"https://APP.copia.io/o/r.git",
		"https://app.copia.io:443/o/r.git",
	} {
		got := fill(target)
		assert.Contains(t, got, "username=token\n", target)
		assert.Contains(t, got, "password=test-token\n", target)
		assert.NotContains(t, got, "user-secret", target)
	}
	for _, target := range []string{
		"https://other.example/o/r.git",
		"https://app.copia.io.evil.example/o/r.git",
		"http://app.copia.io/o/r.git",
		"https://app.copia.io:8443/o/r.git",
	} {
		got := fill(target)
		assert.NotContains(t, got, "test-token", target)
		assert.Contains(t, got, "password=user-secret\n", target)
	}

	for _, action := range []string{"approve", "reject"} {
		assert.Empty(t, credential(action, "url=https://app.copia.io/o/r.git\nusername=token\npassword=test-token\n\n"), action)
	}
}

// gitBackend serves the repositories under root through git http-backend.
func gitBackend(t *testing.T, root string) http.Handler {
	t.Helper()
	// Run git-http-backend directly: the CGI environment has no
	// GIT_EXEC_PATH, and some git packages (Alpine) only ship it with
	// git-daemon.
	backend := filepath.Join(runGit(t, "--exec-path"), "git-http-backend")
	return &cgi.Handler{
		Path: backend,
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1"},
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="copia"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// trustTestCert makes git trust the certificate httptest.NewTLSServer uses
// (the same for every test server) through a GIT_CONFIG_* entry, which
// CloneRun must keep.
func trustTestCert(t *testing.T, srv *httptest.Server) {
	t.Helper()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	require.NoError(t, os.WriteFile(caFile, pemBytes, 0o600))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "http.sslCAInfo")
	t.Setenv("GIT_CONFIG_VALUE_0", caFile)
}

var testBasicAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte("token:test-token"))

func TestCloneRun_TokenToConfiguredHost(t *testing.T) {
	isolateGit(t)
	backend := gitBackend(t, newBareRepo(t))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != testBasicAuth {
			unauthorized(w)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	defer srv.Close()
	trustTestCert(t, srv)
	work := t.TempDir()
	t.Chdir(work)
	ios, _, _, _ := iostreams.Test()

	host := strings.TrimPrefix(srv.URL, "https://")
	err := CloneRun(&CloneOptions{IO: ios, Host: host, Token: "test-token", Repo: "my-org/my-repo"})
	require.NoError(t, err)
	assert.Equal(t, srv.URL+"/my-org/my-repo.git", storedOrigin(t, filepath.Join(work, "my-repo")))
}

// TestCloneRun_RedirectGetsNoToken pins that a redirect from the configured
// host to another origin does not carry the token: not on the redirected
// request and not on the requests git then sends to the new origin. The
// configured host may ask for the token before redirecting (authFirst), and
// the other origin may serve the repository anonymously or ask for
// credentials (otherAuth), which git must then have none for.
func TestCloneRun_RedirectGetsNoToken(t *testing.T) {
	for _, tt := range []struct{ authFirst, otherAuth bool }{
		{false, false}, {true, false}, {false, true}, {true, true},
	} {
		t.Run(fmt.Sprintf("authFirst=%v/otherAuth=%v", tt.authFirst, tt.otherAuth), func(t *testing.T) {
			isolateGit(t)
			backend := gitBackend(t, newBareRepo(t))

			var mu sync.Mutex
			var leaked []string
			other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if auth := r.Header.Get("Authorization"); auth != "" {
					mu.Lock()
					leaked = append(leaked, r.Method+" "+r.URL.Path+": "+auth)
					mu.Unlock()
				} else if tt.otherAuth {
					unauthorized(w)
					return
				}
				backend.ServeHTTP(w, r)
			}))
			defer other.Close()

			var authed atomic.Bool
			configured := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == testBasicAuth {
					authed.Store(true)
				} else if tt.authFirst {
					unauthorized(w)
					return
				}
				http.Redirect(w, r, other.URL+r.URL.RequestURI(), http.StatusFound)
			}))
			defer configured.Close()

			trustTestCert(t, configured)
			work := t.TempDir()
			t.Chdir(work)
			ios, _, _, stderr := iostreams.Test()

			host := strings.TrimPrefix(configured.URL, "https://")
			err := CloneRun(&CloneOptions{IO: ios, Host: host, Token: "test-token", Repo: "my-org/my-repo"})
			switch {
			case tt.otherAuth:
				// git has no credential for the other origin and may not prompt.
				require.Error(t, err, stderr.String())
			case !tt.authFirst:
				// The anonymous redirect is followed, without the token.
				require.NoError(t, err, stderr.String())
			}
			// With authFirst and an anonymous other origin, git POSTs to the
			// configured host again, which redirects, so the clone fails;
			// only the leak check matters there.
			assert.Equal(t, tt.authFirst, authed.Load(), "token sent to the configured host")
			mu.Lock()
			defer mu.Unlock()
			assert.Empty(t, leaked, "Authorization sent to the other origin")
		})
	}
}
