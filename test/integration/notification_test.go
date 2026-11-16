//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	notiflist "github.com/qubernetic/copia-cli/pkg/cmd/notification/list"
)

// The notification tests ask for one notification: the server takes about two
// seconds per notification it returns, so the default of 30 took up to a
// minute (#447).
func TestNotificationList_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &notiflist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      1,
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
		Limit:      1,
	}

	err := notiflist.ListRun(opts)
	if err != nil {
		// Copia/Gitea server returns 500 for ?all=true on some versions
		t.Skipf("Skipping --all test: server returned error: %v", err)
	}
}
