//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	notiflist "github.com/qubernetic/copia-cli/pkg/cmd/notification/list"
)

func TestNotificationList_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &notiflist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      30,
	}

	err := notiflist.ListRun(opts)
	require.NoError(t, err)
}

func TestNotificationList_All(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &notiflist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		All:        true,
		Limit:      30,
	}

	err := notiflist.ListRun(opts)
	if err != nil {
		// Copia/Gitea server returns 500 for ?all=true on some versions
		t.Skipf("Skipping --all test: server returned error: %v", err)
	}
}
