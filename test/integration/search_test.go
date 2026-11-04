//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	searchissues "github.com/qubernetic/copia-cli/pkg/cmd/search/issues"
	searchrepos "github.com/qubernetic/copia-cli/pkg/cmd/search/repos"
)

func TestSearchRepos_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &searchrepos.SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Query:      "integration",
		Limit:      5,
	}

	err := searchrepos.SearchRun(opts)
	require.NoError(t, err)
	assertOutputContains(t, stdout.String(), "integration")
}

func TestSearchIssues_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &searchissues.SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Query:      "lifecycle",
		Limit:      5,
	}

	err := searchissues.SearchRun(opts)
	require.NoError(t, err)
}
