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
	armoredKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmDMEaS0ZRhYJKwYBBAHaRw8BAQdAc29tZWtleW1hdGVyaWFsZm9ydGVzdHM\n=AbCd\n-----END PGP PUBLIC KEY BLOCK-----\n"
	armoredSig = "-----BEGIN PGP SIGNATURE-----\n\niHUEABYKAB0WIQRzaWduYXR1cmVtYXRlcmlhbGZvcnRlc3Rz\n=EfGh\n-----END PGP SIGNATURE-----\n"
)

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
		keyStdin bool
		sig      string
		sigStdin bool
		wantBody map[string]any
	}{
		{name: "a key file", wantBody: map[string]any{"armored_public_key": armoredKey}},
		{name: "the key from standard input", keyStdin: true, wantBody: map[string]any{"armored_public_key": armoredKey}},
		{
			name:     "with --signature",
			sig:      armoredSig,
			wantBody: map[string]any{"armored_public_key": armoredKey, "armored_signature": armoredSig},
		},
		{
			name:     "with the signature from standard input",
			sig:      armoredSig,
			sigStdin: true,
			wantBody: map[string]any{"armored_public_key": armoredKey, "armored_signature": armoredSig},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("POST", "/api/v1/user/gpg_keys"), httpmock.StringResponse(http.StatusCreated, `{"id":4}`))

			ios, stdin, stdout, stderr := iostreams.Test()
			keyFile := "-"
			if tt.keyStdin {
				stdin.WriteString(armoredKey)
			} else {
				keyFile = writeFile(t, "key.asc", armoredKey)
			}
			opts := newOpts(reg, ios, keyFile)
			if tt.sigStdin {
				stdin.WriteString(tt.sig)
				opts.SignatureFile = "-"
			} else if tt.sig != "" {
				opts.SignatureFile = writeFile(t, "sig.asc", tt.sig)
			}

			require.NoError(t, AddRun(opts))
			assert.Equal(t, "GPG key added to your account\n", stdout.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, tt.wantBody, postedBody(t, reg))
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
			assert.Equal(t, "application/json", reg.Requests[0].Header.Get("Content-Type"))
		})
	}
}

func TestAddRun_RefusedBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		sig       string
		bothStdin bool
		wantErr   string
	}{
		{
			name:      "the key and the signature both from standard input",
			bothStdin: true,
			wantErr:   "the key file and the signature file cannot both be standard input",
		},
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
			wantErr: "the key file holds a private key; add the public key (gpg --armor --export <ID>)",
		},
		{
			name:    "a signature that is not armored",
			key:     armoredKey,
			sig:     "iHUEABYKAB0binarysignature",
			wantErr: "the signature is not ASCII-armored; create it with gpg -a --detach-sig",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			ios, stdin, stdout, _ := iostreams.Test()
			opts := newOpts(reg, ios, "-")
			if tt.bothStdin {
				stdin.WriteString(armoredKey)
				opts.SignatureFile = "-"
			} else {
				opts.KeyFile = writeFile(t, "key.asc", tt.key)
				if tt.sig != "" {
					opts.SignatureFile = writeFile(t, "sig.asc", tt.sig)
				}
			}
			err := AddRun(opts)
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
		sig     bool
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
			wantErr: "failed to add GPG key (HTTP 404): None of the emails attached to the GPG key could be found. It may still be added if you provide a valid signature for the token: " + token + "\n" +
				"Sign the token with the key and add the key with the signature:\n" +
				"  copia-cli api user/gpg_key_token | gpg -a --detach-sig > sig.asc\n" +
				"  copia-cli gpg-key add <key-file> --signature sig.asc",
		},
		{
			name:    "a signature that does not match",
			sig:     true,
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"The provided GPG key, signature and token do not match or token is out of date. Provide a valid signature for the token: ` + token + `"}`,
			wantErr: "failed to add GPG key (HTTP 422): The provided GPG key, signature and token do not match or token is out of date. Provide a valid signature for the token: " + token,
		},
		{
			name:    "the GPG key feature turned off",
			status:  http.StatusNotFound,
			body:    `{"message":"Not Found"}`,
			wantErr: "failed to add GPG key (HTTP 404): Not Found",
		},
		{
			name:    "a message that quotes the key shows no key content",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"failed to parse gpg key ` + keyLine + `"}`,
			wantErr: "failed to add GPG key (HTTP 422): failed to parse gpg key [key content]",
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
			opts := newOpts(reg, ios, writeFile(t, "key.asc", armoredKey))
			if tt.sig {
				opts.SignatureFile = writeFile(t, "sig.asc", armoredSig)
			}
			err := AddRun(opts)
			require.EqualError(t, err, tt.wantErr)
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

	opts := newOpts(reg, ios, writeFile(t, "key.asc", armoredKey))
	opts.SignatureFile = filepath.Join(t.TempDir(), "missing.asc")
	require.Error(t, AddRun(opts))
	assert.Empty(t, reg.Requests)
}

func TestRedact(t *testing.T) {
	assert.Equal(t, "bad [key content] here", redact("bad mDMEaS0ZRhYJKwYBBAHaRw8B here", "-----BEGIN X-----\nmDMEaS0ZRhYJKwYBBAHaRw8B\n=AbCd\n"))
	// Short lines such as the armor header or the checksum stay.
	assert.Equal(t, "=AbCd", redact("=AbCd", "=AbCd\n"))
}

func TestNewCmdAdd_Args(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdinTTY bool
		wantFile string
		wantSig  string
		wantErr  string
	}{
		{name: "a file", args: []string{"key.asc"}, wantFile: "key.asc"},
		{name: "a file and a signature", args: []string{"key.asc", "-s", "sig.asc"}, wantFile: "key.asc", wantSig: "sig.asc"},
		{name: "--signature", args: []string{"key.asc", "--signature", "-"}, wantFile: "key.asc", wantSig: "-"},
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
			assert.Equal(t, tt.wantSig, got.SignatureFile)
		})
	}
}

func TestNewCmdAdd_Flags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdAdd(&cmdutil.Factory{IOStreams: ios})
	assert.Equal(t, "s", cmd.Flags().Lookup("signature").Shorthand)
	assert.Nil(t, cmd.Flags().Lookup("title"))
	assert.Contains(t, cmd.Long, "copia-cli api user/gpg_key_token | gpg -a --detach-sig > sig.asc")
	assert.Contains(t, cmd.Example, "copia-cli gpg-key add key.asc --signature sig.asc")
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
