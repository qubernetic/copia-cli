//go:build integration

package integration

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gpgkeydelete "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/delete"
	gpgkeylist "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// Copia adds a GPG key only when one of its emails is a verified email of
// the account, and the tests never read the account's email, so no test
// adds a key: gpg-key add, and gpg-key delete of an existing key, are
// covered by their unit tests.

func TestGPGKeyDelete_NotFound(t *testing.T) {
	env := loadTestEnv(t)

	// A random key ID, which no key of the account has: the command reads
	// every page of the list and refuses before any DELETE.
	b := make([]byte, 8)
	_, err := rand.Read(b)
	require.NoError(t, err, "random key ID")
	keyID := strings.ToUpper(hex.EncodeToString(b))

	ios, out, _ := testIO()
	err = gpgkeydelete.DeleteRun(&gpgkeydelete.DeleteOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		KeyID: keyID,
	})
	require.ErrorContains(t, err, "either the GPG key is not found or it is not owned by you", "gpg-key delete of an unknown key")
	assertOutputEmpty(t, out.String())

	// The list still has no key with that ID.
	ios, out, _ = testIO()
	err = gpgkeylist.ListRun(&gpgkeylist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		JSON: cmdutil.JSONFlags{
			Fields: []string{"key_id"},
			JQ:     `[.[] | select(.key_id == "` + keyID + `")] | length`,
		},
	})
	require.NoError(t, err, "gpg-key list --json failed")
	assertOutputEqual(t, out.String(), "0\n")
}

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
