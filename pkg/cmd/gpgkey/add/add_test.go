package add

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	privateKeyErr = "the key file holds a private key; add the public key (gpg --armor --export <ID>)"
	armorHintErr  = "the GPG key you are trying to upload might not be in ASCII-armored format.\n" +
		"Find your GPG key ID with:    gpg --list-keys\n" +
		"Then add it to your account:  gpg --armor --export <ID> | copia-cli gpg-key add -"
)

const armoredKey ="-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmDMEaS0ZRhYJKwYBBAHaRw8BAQdAc29tZWtleW1hdGVyaWFsZm9ydGVzdHM\n=AbCd\n-----END PGP PUBLIC KEY BLOCK-----\n"

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams, keyFile string) *AddOptions {
	return &AddOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		KeyFile:    keyFile,
	}
}

// postedBody returns the JSON body of the POST the registry got.
func postedBody(t *testing.T, reg *httpmock.Registry) map[string]any {
	t.Helper()
	require.Len(t, reg.Requests, 1)
	data, err := io.ReadAll(reg.Requests[0].Body)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(data, &body))
	return body
}

func TestAddRun_Adds(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		keyStdin bool
	}{
		{name: "a key file", key: armoredKey},
		{name: "the key from standard input", key: armoredKey, keyStdin: true},
		{name: "two public key blocks", key: armoredKey + armoredKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("POST", "/api/v1/user/gpg_keys"), httpmock.StringResponse(http.StatusCreated, `{"id":4}`))

			ios, stdin, stdout, stderr := iostreams.Test()
			keyFile := "-"
			if tt.keyStdin {
				stdin.WriteString(tt.key)
			} else {
				keyFile = writeFile(t, "key.asc", tt.key)
			}
			opts := newOpts(reg, ios, keyFile)

			require.NoError(t, AddRun(opts))
			assert.Equal(t, "GPG key added to your account\n", stdout.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, map[string]any{"armored_public_key": tt.key}, postedBody(t, reg))
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
			assert.Equal(t, "application/json", reg.Requests[0].Header.Get("Content-Type"))
		})
	}
}

func TestAddRun_RefusedBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{
			name: "a key that is not armored",
			key:  "mDMEaS0ZRhYJKwYBBAHaRw8BAQdAbinarykey\n",
			wantErr: "the GPG key you are trying to upload might not be in ASCII-armored format.\n" +
				"Find your GPG key ID with:    gpg --list-keys\n" +
				"Then add it to your account:  gpg --armor --export <ID> | copia-cli gpg-key add -",
		},
		{
			name:    "a private key",
			key:     "-----BEGIN PGP PRIVATE KEY BLOCK-----\n\nlFgEaS0ZRhYJKwYBBAHaRw8BAQdAsecret\n-----END PGP PRIVATE KEY BLOCK-----\n",
			wantErr: privateKeyErr,
		},
		{
			name:    "an OpenSSH private key",
			key:     "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmU\n-----END OPENSSH PRIVATE KEY-----\n",
			wantErr: privateKeyErr,
		},
		{
			name:    "a PKCS#8 private key",
			key:     "-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIBy\n-----END PRIVATE KEY-----\n",
			wantErr: privateKeyErr,
		},
		{
			name:    "an RSA private key",
			key:     "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----\n",
			wantErr: privateKeyErr,
		},
		{
			name:    "a legacy PGP secret key",
			key:     "-----BEGIN PGP SECRET KEY BLOCK-----\n\nlFgEaS0ZRhYJKwYBBAHaRw8B\n-----END PGP SECRET KEY BLOCK-----\n",
			wantErr: privateKeyErr,
		},
		{
			name:    "a public key followed by a private key",
			key:     armoredKey + "-----BEGIN PGP PRIVATE KEY BLOCK-----\n\nlFgEaS0ZRhYJKwYBBAHaRw8B\n-----END PGP PRIVATE KEY BLOCK-----\n",
			wantErr: privateKeyErr,
		},
		{
			name:    "a signature instead of a key",
			key:     "-----BEGIN PGP SIGNATURE-----\n\niHUEABYKAB0WIQRzaWdu\n-----END PGP SIGNATURE-----\n",
			wantErr: armorHintErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			ios, _, stdout, _ := iostreams.Test()
			err := AddRun(newOpts(reg, ios, writeFile(t, "key.asc", tt.key)))
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, reg.Requests)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestAddRun_GiteaError(t *testing.T) {
	const token = "1f2e3d4c5b6a79881f2e3d4c5b6a79881f2e3d4c5b6a79881f2e3d4c5b6a7988"
	keyLine := strings.Split(armoredKey, "\n")[2]
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "a duplicate key ID",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"A key with the same id already exists","url":"https://app.copia.io/api/swagger"}`,
			wantErr: "the key already exists in your account",
		},
		{
			name:   "a key without a verified email",
			status: http.StatusNotFound,
			body:   `{"message":"None of the emails attached to the GPG key could be found. It may still be added if you provide a valid signature for the token: ` + token + `"}`,
			// Gitea's advice to sign the token cannot be followed on
			// Copia, which has no token route, so it is left out.
			wantErr: "failed to add GPG key (HTTP 404): None of the emails attached to the GPG key could be found.\n" +
				"Copia adds a GPG key only when one of its emails is a verified email of your account;\n" +
				"add and verify that email in your Copia settings, then add the key again",
		},
		{
			name:    "the GPG key feature turned off",
			status:  http.StatusNotFound,
			body:    `{"message":"Not Found"}`,
			wantErr: "failed to add GPG key (HTTP 404): Not Found",
		},
		{
			name:    "a message that quotes a key line is withheld",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"failed to parse gpg key ` + keyLine + `"}`,
			wantErr: "failed to add GPG key (HTTP 422)",
		},
		{
			name:    "a message that quotes the key joined on one line is withheld",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"bad key ` + strings.Join(strings.Fields(armoredKey), "") + `"}`,
			wantErr: "failed to add GPG key (HTTP 422)",
		},
		{
			name:    "a message that quotes part of a key line is withheld",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"bad key …` + keyLine[10:30] + `…"}`,
			wantErr: "failed to add GPG key (HTTP 422)",
		},
		{
			name:    "a long message is cut",
			status:  http.StatusInternalServerError,
			body:    `{"message":"` + strings.Repeat("m", 10<<10) + `"}`,
			wantErr: "failed to add GPG key (HTTP 500): " + strings.Repeat("m", cmdutil.MaxAPIMessage),
		},
		{
			name:    "a 1 MiB body",
			status:  http.StatusInternalServerError,
			body:    `{"message":"` + strings.Repeat("m", 100<<10) + `","pad":"` + strings.Repeat("p", 1<<20) + `"}`,
			wantErr: "failed to add GPG key (HTTP 500)",
		},
		{
			name:    "no message",
			status:  http.StatusInternalServerError,
			body:    ``,
			wantErr: "failed to add GPG key (HTTP 500)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("POST", "/api/v1/user/gpg_keys"), httpmock.StringResponse(tt.status, tt.body))

			ios, _, stdout, _ := iostreams.Test()
			err := AddRun(newOpts(reg, ios, writeFile(t, "key.asc", armoredKey)))
			require.EqualError(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), token)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestAddRun_MissingFile(t *testing.T) {
	reg := &httpmock.Registry{}
	ios, _, _, _ := iostreams.Test()
	err := AddRun(newOpts(reg, ios, filepath.Join(t.TempDir(), "missing.asc")))
	require.Error(t, err)
	assert.Empty(t, reg.Requests)
}

func TestQuotesKey(t *testing.T) {
	body := strings.Split(armoredKey, "\n")[2]
	// The key re-wrapped at 20 columns, as a server might quote it.
	var wrapped []string
	for s := body; s != ""; {
		n := min(20, len(s))
		wrapped = append(wrapped, s[:n])
		s = s[n:]
	}
	tests := []struct {
		name    string
		message string
		key     string
		want    bool
	}{
		{name: "a key line", message: "bad " + body + " here", key: armoredKey, want: true},
		{name: "the key re-wrapped", message: "bad " + strings.Join(wrapped, "\n"), key: armoredKey, want: true},
		{name: "the key joined on one line", message: strings.Join(strings.Fields(armoredKey), ""), key: armoredKey, want: true},
		{name: "16 bytes of the key", message: "x" + body[5:21] + "x", key: armoredKey, want: true},
		{name: "a single-line key file", message: "bad " + body[:16], key: body, want: true},
		{name: "15 bytes of the key", message: "x" + body[5:20] + "x", key: armoredKey, want: false},
		{name: "the armor header", message: "no -----BEGIN PGP PUBLIC KEY BLOCK----- found", key: armoredKey, want: false},
		{name: "an unrelated message", message: "A key with the same id already exists", key: armoredKey, want: false},
		{name: "a key shorter than 16 bytes", message: "=AbCd", key: "=AbCd\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, quotesKey(tt.message, tt.key))
		})
	}
}

func TestNewCmdAdd_Args(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdinTTY bool
		wantFile string
		wantErr  string
	}{
		{name: "a file", args: []string{"key.asc"}, wantFile: "key.asc"},
		{name: "no --signature", args: []string{"key.asc", "--signature", "sig.asc"}, wantErr: "unknown flag: --signature"},
		{name: "a dash", args: []string{"-"}, stdinTTY: true, wantFile: "-"},
		{name: "no argument with piped stdin", wantFile: "-"},
		{name: "no argument on a terminal", stdinTTY: true, wantErr: "GPG key file missing"},
		{name: "two arguments", args: []string{"a", "b"}, wantErr: "accepts at most 1 arg(s), received 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.stdinTTY)
			f := &cmdutil.Factory{IOStreams: ios}

			var got *AddOptions
			cmd := newCmdAdd(f, func(opts *AddOptions) error {
				got = opts
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFile, got.KeyFile)
		})
	}
}

func TestNewCmdAdd_Flags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdAdd(&cmdutil.Factory{IOStreams: ios})
	assert.Nil(t, cmd.Flags().Lookup("signature"))
	assert.Nil(t, cmd.Flags().Lookup("title"))
	assert.Equal(t, "add [<key-file>]", cmd.Use)
	assert.Contains(t, cmd.Long, "only when one of its emails is a verified email of your\naccount")
	assert.NotContains(t, cmd.Long, "gpg_key_token")
}

func TestNewCmdAdd_Runs(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", "/api/v1/user/gpg_keys"), httpmock.StringResponse(http.StatusCreated, `{"id":4}`))

	ios, _, stdout, _ := iostreams.Test()
	cmd := NewCmdAdd(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "flag-token"})
	cmd.SetArgs([]string{writeFile(t, "key.asc", armoredKey)})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "GPG key added to your account\n", stdout.String())
	assert.Equal(t, "token flag-token", reg.Requests[0].Header.Get("Authorization"))
}
