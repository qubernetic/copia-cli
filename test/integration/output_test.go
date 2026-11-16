//go:build integration

package integration

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/internal/copiacmd"
)

// TestOutput_ConfigJSON runs issue list through the root command with a
// temporary config file that holds output: json and no --json flag.
func TestOutput_ConfigJSON(t *testing.T) {
	env := loadTestEnv(t)
	// An issue, so the table output is not empty either.
	createIssue(t, env, "[integration-test] output")

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_OUTPUT", "")
	cfg := &config.Config{
		Hosts:  map[string]*config.HostConfig{env.Host: {Token: env.Token}},
		Output: "json",
	}
	require.NoError(t, config.Save(filepath.Join(dir, "copia", "config.yml"), cfg), "saving the temporary config failed")

	args := []string{"issue", "list", "-R", env.Owner + "/" + env.Repo, "--state", "all", "--limit", "5"}

	ios, stdout, _ := testIO()
	require.NoError(t, copiacmd.Execute(copiacmd.NewFactory(ios), args), "issue list with output json failed")
	assertOutputContains(t, stdout.String(), "[")
	assert.True(t, json.Valid(stdout.Bytes()), "issue list with output json printed invalid JSON")

	t.Setenv("COPIA_OUTPUT", "table")
	ios, stdout, _ = testIO()
	require.NoError(t, copiacmd.Execute(copiacmd.NewFactory(ios), args), "issue list with COPIA_OUTPUT=table failed")
	assert.False(t, json.Valid(stdout.Bytes()), "issue list with COPIA_OUTPUT=table printed JSON")
}
