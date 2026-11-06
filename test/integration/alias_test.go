//go:build integration

package integration

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/internal/copiacmd"
)

// TestAlias_IssueList runs aliases for issue list -L 1 through the root
// command with a temporary config file, once with appended arguments and
// once with placeholders.
func TestAlias_IssueList(t *testing.T) {
	env := loadTestEnv(t)
	// -L 1 prints one issue only if the run repository has one.
	createIssue(t, env, "[integration-test] alias")

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("COPIA_HOST", "")
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_OUTPUT", "")
	cfg := &config.Config{
		Hosts: map[string]*config.HostConfig{env.Host: {Token: env.Token}},
		Aliases: map[string]string{
			"il":  "issue list -L 1",
			"ilr": "issue list -L 1 -R $1 --state $2 --json number --jq length",
		},
	}
	require.NoError(t, config.Save(filepath.Join(dir, "copia", "config.yml"), cfg), "saving the temporary config failed")
	repo := env.Owner + "/" + env.Repo

	ios, stdout, _ := testIO()
	args := []string{"il", "-R", repo, "--state", "all", "--json", "number", "--jq", "length"}
	require.NoError(t, copiacmd.Execute(copiacmd.NewFactory(ios), args), "the il alias failed")
	assertOutputEqual(t, stdout.String(), "1\n")

	ios, stdout, _ = testIO()
	require.NoError(t, copiacmd.Execute(copiacmd.NewFactory(ios), []string{"ilr", repo, "all"}), "the ilr alias failed")
	assertOutputEqual(t, stdout.String(), "1\n")
}
