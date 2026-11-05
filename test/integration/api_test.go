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
	assertOutputContains(t, stdout.String(), "login")
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
	assertOutputContains(t, stdout.String(), env.Repo)
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
	require.True(t, found, "no blank line after the headers")
	statusLine, _, _ := strings.Cut(head, "\n")
	assertOutputContains(t, statusLine, " 200 ")

	var v struct {
		Version string `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &v))
	require.True(t, v.Version != "", "empty version")
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
	assert.ErrorContains(t, err, "404")
}
