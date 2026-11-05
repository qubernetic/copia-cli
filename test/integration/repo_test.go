//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repolist "github.com/qubernetic/copia-cli/pkg/cmd/repo/list"
	repoview "github.com/qubernetic/copia-cli/pkg/cmd/repo/view"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func TestRepoList_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &repolist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      5,
	}

	err := repolist.ListRun(opts)
	require.NoError(t, err)
	assertOutputNotEmpty(t, stdout.String())
}

func TestRepoList_JSON(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &repolist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      2,
		JSON:       cmdutil.JSONFlags{Fields: []string{"full_name"}},
	}

	err := repolist.ListRun(opts)
	require.NoError(t, err)
	assertOutputContains(t, stdout.String(), "full_name")
}

func TestRepoView_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &repoview.ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
	}

	err := repoview.ViewRun(opts)
	require.NoError(t, err)
	output := stdout.String()
	assertOutputContains(t, output, env.Owner+"/"+env.Repo)
	assertOutputContains(t, output, "Default branch")
}

func TestRepoView_NotFound(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &repoview.ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      "nonexistent-org-xyz",
		Repo:       "nonexistent-repo-xyz",
	}

	err := repoview.ViewRun(opts)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}
