//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	prlist "github.com/qubernetic/copia-cli/pkg/cmd/pr/list"
)

func TestPR_List(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &prlist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		State:      "all",
		Limit:      5,
	}

	err := prlist.ListRun(opts)
	require.NoError(t, err)
}
