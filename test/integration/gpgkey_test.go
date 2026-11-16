//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	gpgkeylist "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// Copia adds a GPG key only when one of its emails is a verified email of
// the account, and the tests never read the account's email, so no test
// adds a key: gpg-key add is covered by its unit tests.

func TestGPGKeyList_Run(t *testing.T) {
	env := loadTestEnv(t)

	// Without a terminal: one line per key, or the no-keys message on
	// standard error; either way no error.
	ios, _, _ := testIO()
	err := gpgkeylist.ListRun(&gpgkeylist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
	})
	require.NoError(t, err, "gpg-key list failed")

	// --json gives an array, also for an account without keys.
	ios, out, _ := testIO()
	err = gpgkeylist.ListRun(&gpgkeylist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		JSON: cmdutil.JSONFlags{
			Fields: []string{"id", "key_id", "verified"},
			JQ:     `type`,
		},
	})
	require.NoError(t, err, "gpg-key list --json failed")
	assertOutputEqual(t, out.String(), "array\n")
}
