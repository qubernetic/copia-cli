//go:build integration

package integration

import (
	"net/http"
	"testing"

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
	assertOutputNotEmpty(t, stdout.String())
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
	assertOutputContains(t, stdout.String(), env.Owner)
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
