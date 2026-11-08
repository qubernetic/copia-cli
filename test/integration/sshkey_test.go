//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	sshkeyadd "github.com/qubernetic/copia-cli/pkg/cmd/sshkey/add"
	sshkeylist "github.com/qubernetic/copia-cli/pkg/cmd/sshkey/list"
)

func TestSSHKey_AddAndList(t *testing.T) {
	env := loadTestEnv(t)
	key := newRunKey(t, env, "sshadd")

	// The title comes from the key's comment, as ssh-keygen writes it.
	keyFile := filepath.Join(t.TempDir(), "id_ed25519.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte(key.Public+" "+key.Title+"\n"), 0o600), "write the public key file")

	add := func() string {
		t.Helper()
		ios, out, _ := testIO()
		err := sshkeyadd.AddRun(&sshkeyadd.AddOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			KeyFile: keyFile,
		})
		require.NoError(t, err, "ssh-key add failed")
		return out.String()
	}
	assertOutputEqual(t, add(), "Public key added to your account\n")

	ios, out, _ := testIO()
	err := sshkeylist.ListRun(&sshkeylist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
	})
	require.NoError(t, err, "ssh-key list failed")
	// Without a terminal: title, key, created_at, id, type.
	assertOutputContainsSecret(t, out.String(), key.Title+"\t"+key.Public+"\t")
	assertOutputContains(t, out.String(), "\tauthentication\n")

	// The same key again is found in the list and not sent.
	assertOutputEqual(t, add(), "Public key already exists on your account\n")
}
