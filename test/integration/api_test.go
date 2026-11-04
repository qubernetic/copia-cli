//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicmd "github.com/qubernetic/copia-cli/pkg/cmd/api"
)

func TestAPI_GetUser(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &apicmd.APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Path:       "/user",
	}

	err := apicmd.APIRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "login")
	t.Logf("API /user: %s", stdout.String()[:100])
}

func TestAPI_GetRepo(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &apicmd.APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Path:       fmt.Sprintf("/repos/%s/%s", env.Owner, env.Repo),
	}

	err := apicmd.APIRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), env.Repo)
	t.Logf("API repo: %s", stdout.String()[:100])
}

func TestAPI_Version(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &apicmd.APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Path:       "/version",
		Include:    true,
	}

	err := apicmd.APIRun(opts)
	require.NoError(t, err)

	head, body, found := strings.Cut(stdout.String(), "\n\n")
	require.True(t, found, "no blank line after the headers: %q", stdout.String())
	statusLine, _, _ := strings.Cut(head, "\n")
	assert.Contains(t, statusLine, " 200 ")

	var v struct {
		Version string `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &v), "body: %q", body)
	require.NotEmpty(t, v.Version)
	t.Logf("Copia version (GET /version): %s", v.Version)
}

func TestAPI_NotFound(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &apicmd.APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Path:       "/repos/nonexistent/nonexistent",
	}

	err := apicmd.APIRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
