package clone

import (
	"encoding/base64"
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

var wantHeader = "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("token:"+testToken))

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
			args, env := cloneCommand(&CloneOptions{Host: "app.copia.io", Token: testToken, Repo: tt.repo}, 0)
			assert.Equal(t, []string{"clone", "--", tt.wantURL}, args)
			assert.Equal(t, []string{"GIT_TERMINAL_PROMPT=0"}, env)
			assertNoToken(t, args, env)
		})
	}
}

func TestCloneCommand_HeaderForConfiguredHost(t *testing.T) {
	tests := []struct {
		host    string
		repo    string
		dir     string
		wantURL string
		wantKey string
	}{
		{host: "app.copia.io", repo: "my-org/my-repo", wantURL: "https://app.copia.io/my-org/my-repo.git", wantKey: "http.https://app.copia.io/.extraHeader"},
		{host: "app.copia.io", repo: "https://APP.copia.io/o/r.git", wantURL: "https://APP.copia.io/o/r.git", wantKey: "http.https://app.copia.io/.extraHeader"},
		{host: "app.copia.io", repo: "https://app.copia.io:443/o/r.git", dir: "target", wantURL: "https://app.copia.io:443/o/r.git", wantKey: "http.https://app.copia.io/.extraHeader"},
		{host: "App.Copia.io", repo: "https://app.copia.io/o/r.git", wantURL: "https://app.copia.io/o/r.git", wantKey: "http.https://app.copia.io/.extraHeader"},
		{host: "copia.local:8443", repo: "https://copia.local:8443/o/r.git", wantURL: "https://copia.local:8443/o/r.git", wantKey: "http.https://copia.local:8443/.extraHeader"},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			args, env := cloneCommand(&CloneOptions{Host: tt.host, Token: testToken, Repo: tt.repo, Dir: tt.dir}, 0)

			wantArgs := []string{"clone", "--", tt.wantURL}
			if tt.dir != "" {
				wantArgs = append(wantArgs, tt.dir)
			}
			assert.Equal(t, wantArgs, args)
			for _, a := range args {
				assert.NotContains(t, a, testToken)
			}
			assert.Equal(t, []string{
				"GIT_TERMINAL_PROMPT=0",
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=" + tt.wantKey,
				"GIT_CONFIG_VALUE_0=" + wantHeader,
			}, env)
		})
	}
}

func TestCloneCommand_KeepsExistingConfigEntries(t *testing.T) {
	_, env := cloneCommand(&CloneOptions{Host: "app.copia.io", Token: testToken, Repo: "o/r"}, 3)
	assert.Equal(t, []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_3=http.https://app.copia.io/.extraHeader",
		"GIT_CONFIG_VALUE_3=" + wantHeader,
	}, env)
}

func TestCloneCommand_NoTokenConfigured(t *testing.T) {
	for _, repo := range []string{"my-org/my-repo", "https://app.copia.io/o/r.git"} {
		args, env := cloneCommand(&CloneOptions{Host: "app.copia.io", Repo: repo}, 0)
		assert.Equal(t, []string{"GIT_TERMINAL_PROMPT=0"}, env)
		for _, s := range append(args, env...) {
			assert.False(t, strings.Contains(s, "extraHeader") || strings.Contains(s, "Authorization") || strings.Contains(s, "@app.copia.io"), s)
		}
	}
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
