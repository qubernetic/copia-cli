package clone

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCloneURL_FullURL(t *testing.T) {
	got := buildCloneURL("app.copia.io", "https://app.copia.io/my-org/my-repo.git")
	assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", got)
}

func TestBuildCloneURL_OwnerRepo(t *testing.T) {
	got := buildCloneURL("app.copia.io", "my-org/my-repo")
	assert.Equal(t, "https://app.copia.io/my-org/my-repo.git", got)
}

func TestBuildCloneURL_SSHPassthrough(t *testing.T) {
	got := buildCloneURL("", "git@app.copia.io:my-org/my-repo.git")
	assert.Equal(t, "git@app.copia.io:my-org/my-repo.git", got)
}

func TestBuildCloneURL_OtherSchemePassthrough(t *testing.T) {
	got := buildCloneURL("app.copia.io", "http://app.copia.io/my-org/my-repo.git")
	assert.Equal(t, "http://app.copia.io/my-org/my-repo.git", got)
}

const testToken = "s3cret-tok"

// helperEnv is the environment cloneCommand adds for a token sent to the
// host whose credential config key is key, after existing entries.
func helperEnv(key string, existing int) []string {
	return []string{
		"GIT_TERMINAL_PROMPT=0",
		"COPIA_CLONE_TOKEN=" + testToken,
		fmt.Sprintf("GIT_CONFIG_COUNT=%d", existing+2),
		fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", existing, key),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=", existing),
		fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", existing+1, key),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", existing+1, credentialHelper),
	}
}

func assertNoToken(t *testing.T, args, env []string) {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString([]byte("token:" + testToken))
	for _, s := range append(append([]string{}, args...), env...) {
		assert.NotContains(t, s, testToken)
		assert.NotContains(t, s, encoded)
	}
}

func TestCloneCommand_TokenOnlyForConfiguredHost(t *testing.T) {
	tests := []struct {
		repo    string
		wantURL string
	}{
		{repo: "https://other.example/o/r.git", wantURL: "https://other.example/o/r.git"},
		{repo: "https://app.copia.io.evil.example/o/r.git", wantURL: "https://app.copia.io.evil.example/o/r.git"},
		{repo: "http://app.copia.io/o/r.git", wantURL: "http://app.copia.io/o/r.git"},
		{repo: "https://app.copia.io:8443/o/r.git", wantURL: "https://app.copia.io:8443/o/r.git"},
		{repo: "git@app.copia.io:o/r.git", wantURL: "git@app.copia.io:o/r.git"},
		{repo: "https://token@app.copia.io@other.example/o/r.git", wantURL: "https://token@app.copia.io@other.example/o/r.git"},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			args, env, err := cloneCommand(&CloneOptions{Host: "app.copia.io", Token: testToken, Repo: tt.repo}, 0)
			require.NoError(t, err)
			assert.Equal(t, []string{"clone", "--", tt.wantURL}, args)
			assert.Equal(t, []string{"GIT_TERMINAL_PROMPT=0"}, env)
			assertNoToken(t, args, env)
		})
	}
}

func TestCloneCommand_HelperForConfiguredHost(t *testing.T) {
	tests := []struct {
		host    string
		repo    string
		dir     string
		wantURL string
		wantKey string
	}{
		{host: "app.copia.io", repo: "my-org/my-repo", wantURL: "https://app.copia.io/my-org/my-repo.git", wantKey: "credential.https://app.copia.io.helper"},
		{host: "app.copia.io", repo: "https://APP.copia.io/o/r.git", wantURL: "https://APP.copia.io/o/r.git", wantKey: "credential.https://app.copia.io.helper"},
		{host: "app.copia.io", repo: "https://app.copia.io:443/o/r.git", dir: "target", wantURL: "https://app.copia.io:443/o/r.git", wantKey: "credential.https://app.copia.io.helper"},
		{host: "App.Copia.io", repo: "https://app.copia.io/o/r.git", wantURL: "https://app.copia.io/o/r.git", wantKey: "credential.https://app.copia.io.helper"},
		{host: "copia.local:8443", repo: "https://copia.local:8443/o/r.git", wantURL: "https://copia.local:8443/o/r.git", wantKey: "credential.https://copia.local:8443.helper"},
		{host: "app.copia.io:443", repo: "my-org/my-repo", wantURL: "https://app.copia.io:443/my-org/my-repo.git", wantKey: "credential.https://app.copia.io.helper"},
		{host: "[::1]:8443", repo: "o/r", wantURL: "https://[::1]:8443/o/r.git", wantKey: "credential.https://[::1]:8443.helper"},
		{host: "[::1]", repo: "https://[::1]/o/r.git", wantURL: "https://[::1]/o/r.git", wantKey: "credential.https://[::1].helper"},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			args, env, err := cloneCommand(&CloneOptions{Host: tt.host, Token: testToken, Repo: tt.repo, Dir: tt.dir}, 0)
			require.NoError(t, err)

			wantArgs := []string{"clone", "--", tt.wantURL}
			if tt.dir != "" {
				wantArgs = append(wantArgs, tt.dir)
			}
			assert.Equal(t, wantArgs, args)
			for _, a := range args {
				assert.NotContains(t, a, testToken)
			}
			assert.Equal(t, helperEnv(tt.wantKey, 0), env)
		})
	}
}

func TestCloneCommand_KeepsExistingConfigEntries(t *testing.T) {
	_, env, err := cloneCommand(&CloneOptions{Host: "app.copia.io", Token: testToken, Repo: "o/r"}, 3)
	require.NoError(t, err)
	assert.Equal(t, helperEnv("credential.https://app.copia.io.helper", 3), env)
}

func TestCloneCommand_NoTokenConfigured(t *testing.T) {
	for _, repo := range []string{"my-org/my-repo", "https://app.copia.io/o/r.git"} {
		args, env, err := cloneCommand(&CloneOptions{Host: "app.copia.io", Repo: repo}, 0)
		require.NoError(t, err)
		assert.Equal(t, []string{"GIT_TERMINAL_PROMPT=0"}, env)
		for _, s := range append(args, env...) {
			assert.False(t, strings.Contains(s, "credential") || strings.Contains(s, "COPIA_CLONE_TOKEN") || strings.Contains(s, "@app.copia.io"), s)
		}
	}
}

// A configured host carrying git URL-pattern syntax would widen or shift
// the credential key, so it is refused rather than written into it.
func TestCloneCommand_RejectsInvalidHost(t *testing.T) {
	for _, host := range []string{
		"",
		"*.copia.io",
		"app.*.io",
		"u@app.copia.io",
		"u:p@app.copia.io",
		"app.copia.io/sub",
		"app.copia.io/",
		"app.copia.io?x=1",
		"app.copia.io#frag",
		"app.copia.io:bad",
		"app copia.io",
		"[fe80::1%25eth0]",
		`app.copia.io\sub`,
	} {
		for _, token := range []string{testToken, ""} {
			args, env, err := cloneCommand(&CloneOptions{Host: host, Token: token, Repo: "o/r"}, 0)
			require.Error(t, err, host)
			assert.Contains(t, err.Error(), "invalid host", host)
			assert.Nil(t, args, host)
			assert.Nil(t, env, host)
		}
	}
}

// The helper prints the token as one password= line, so a line break or
// any other character outside printable ASCII is refused before git runs.
func TestCloneCommand_RejectsTokenWithControlCharacters(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "line feed", token: "tok\nhost=other.example"},
		{name: "carriage return", token: "tok\rurl=https://other.example"},
		{name: "tab", token: "tok\tx"},
		{name: "NUL", token: "tok\x00x"},
		{name: "DEL", token: "tok\x7fx"},
		{name: "non-ASCII", token: "tokéx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &CloneOptions{Host: "app.copia.io", Token: tt.token, Repo: "o/r"}
			args, env, err := cloneCommand(opts, 0)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid token")
			assert.NotContains(t, err.Error(), tt.token)
			assert.Nil(t, args)
			for _, e := range env {
				assert.NotContains(t, e, "COPIA_CLONE_TOKEN")
			}

			// CloneRun returns the error before it starts git; opts.IO is
			// nil, so reaching exec would panic.
			require.Error(t, CloneRun(opts))
		})
	}
}

func TestCloneCommand_AcceptsPrintableToken(t *testing.T) {
	var b strings.Builder
	for c := byte(' '); c <= '~'; c++ {
		b.WriteByte(c)
	}
	token := b.String()
	_, env, err := cloneCommand(&CloneOptions{Host: "app.copia.io", Token: token, Repo: "o/r"}, 0)
	require.NoError(t, err)
	assert.Contains(t, env, "COPIA_CLONE_TOKEN="+token)
}

func TestConfigCount(t *testing.T) {
	for value, want := range map[string]int{"": 0, "0": 0, "3": 3} {
		got, err := configCount(value)
		require.NoError(t, err, value)
		assert.Equal(t, want, got, value)
	}
	for _, value := range []string{"bogus", "-2", "99999999999999999999", " 1"} {
		_, err := configCount(value)
		require.Error(t, err, value)
		assert.Contains(t, err.Error(), "GIT_CONFIG_COUNT", value)
	}
}
