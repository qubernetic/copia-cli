//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orglist "github.com/qubernetic/copia-cli/pkg/cmd/org/list"
	orgview "github.com/qubernetic/copia-cli/pkg/cmd/org/view"
)

func TestOrgList_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &orglist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      30,
	}

	err := orglist.ListRun(opts)
	require.NoError(t, err)
	assert.NotEmpty(t, stdout.String())
	t.Logf("Orgs: %s", stdout.String())
}

func TestOrgView_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &orgview.ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Name:       env.Owner,
	}

	err := orgview.ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), env.Owner)
	t.Logf("Org: %s", stdout.String())
}

func TestOrgView_NotFound(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &orgview.ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Name:       "nonexistent-org-xyz-123",
	}

	err := orgview.ViewRun(opts)
	require.Error(t, err)
}
