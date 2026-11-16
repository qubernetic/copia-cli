//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gpgkeyadd "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/add"
	gpgkeylist "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func TestGPGKey_AddAndList(t *testing.T) {
	env := loadTestEnv(t)
	key := newRunGPGKey(t, env, "gpgadd")

	// The signature route: the key's user ID names no email of the account.
	status, token := apiCall(t, env, "read the GPG key token", http.MethodGet, "/user/gpg_key_token", nil)
	require.True(t, assertStatusCode(t, status, http.StatusOK), "the GPG key token could not be read")
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.asc")
	sigFile := filepath.Join(dir, "sig.asc")
	require.NoError(t, os.WriteFile(keyFile, []byte(key.Armored), 0o600), "write the public key file")
	require.NoError(t, os.WriteFile(sigFile, []byte(key.sign(t, string(token))), 0o600), "write the signature file")

	add := func() error {
		t.Helper()
		ios, out, _ := testIO()
		err := gpgkeyadd.AddRun(&gpgkeyadd.AddOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			KeyFile: keyFile, SignatureFile: sigFile,
		})
		if err == nil {
			assertOutputEqual(t, out.String(), "GPG key added to your account\n")
		}
		return err
	}
	require.NoError(t, add(), "gpg-key add --signature failed")

	ios, out, _ := testIO()
	err := gpgkeylist.ListRun(&gpgkeylist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		JSON: cmdutil.JSONFlags{
			Fields: []string{"key_id", "verified"},
			JQ:     fmt.Sprintf(`[.[] | select(.key_id == %q and .verified)] | length`, key.KeyID),
		},
	})
	require.NoError(t, err, "gpg-key list failed")
	assertOutputEqual(t, out.String(), "1\n")

	// The same key again is refused as a duplicate.
	require.ErrorContains(t, add(), "the key already exists in your account", "a second add of the key was not refused as a duplicate")
}
