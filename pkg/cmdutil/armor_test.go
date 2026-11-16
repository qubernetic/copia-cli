package cmdutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	gpgPublic = "-----BEGIN PGP PUBLIC KEY BLOCK-----"
	publicKey = gpgPublic + "\n\nmDMEaS0ZRhYJKwYBBAHaRw8B\n=AbCd\n-----END PGP PUBLIC KEY BLOCK-----\n"
)

func TestCheckArmor(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    ArmorKind
	}{
		{name: "a public key", content: publicKey, want: ArmorPublic},
		{name: "two public keys", content: publicKey + publicKey, want: ArmorPublic},
		{name: "a public key with CRLF line ends", content: gpgPublic + "\r\n\r\nmDMEaS0Z\r\n-----END PGP PUBLIC KEY BLOCK-----\r\n", want: ArmorPublic},
		{name: "a PGP private key", content: "-----BEGIN PGP PRIVATE KEY BLOCK-----\nlFgE\n-----END PGP PRIVATE KEY BLOCK-----\n", want: ArmorPrivate},
		{name: "a legacy PGP secret key", content: "-----BEGIN PGP SECRET KEY BLOCK-----\nlFgE\n-----END PGP SECRET KEY BLOCK-----\n", want: ArmorPrivate},
		{name: "an OpenSSH private key", content: "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEA\n-----END OPENSSH PRIVATE KEY-----\n", want: ArmorPrivate},
		{name: "a PKCS#8 private key", content: "-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIE\n-----END PRIVATE KEY-----\n", want: ArmorPrivate},
		{name: "an encrypted PKCS#8 private key", content: "-----BEGIN ENCRYPTED PRIVATE KEY-----\nMIIF\n-----END ENCRYPTED PRIVATE KEY-----\n", want: ArmorPrivate},
		{name: "an RSA private key", content: "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----\n", want: ArmorPrivate},
		{name: "a public key followed by a private key", content: publicKey + "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl\n-----END OPENSSH PRIVATE KEY-----\n", want: ArmorPrivate},
		{name: "a private key header inside a line", content: publicKey + "x -----BEGIN EC PRIVATE KEY-----\n", want: ArmorPrivate},
		{name: "a lower-case secret header", content: "-----BEGIN pgp secret key block-----\n", want: ArmorPrivate},
		{name: "no armor", content: "mDMEaS0ZRhYJKwYBBAHaRw8BAQdA\n", want: ArmorOther},
		{name: "empty", content: "", want: ArmorOther},
		{name: "a signature", content: "-----BEGIN PGP SIGNATURE-----\niHUE\n-----END PGP SIGNATURE-----\n", want: ArmorOther},
		{name: "an SSH public key block", content: "-----BEGIN SSH2 PUBLIC KEY-----\nAAAA\n-----END SSH2 PUBLIC KEY-----\n", want: ArmorOther},
		{name: "a public key and a signature", content: publicKey + "-----BEGIN PGP SIGNATURE-----\niHUE\n", want: ArmorOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CheckArmor(tt.content, gpgPublic))
		})
	}
}
