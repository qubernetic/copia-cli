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
	pubKey      = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGq1b2xvZ3lUZXN0S2V5Rm9yQ29waWFDbGk"
	ownKeysBody = `[{"id":3,"title":"other","key":"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQOther"}]`
)

func writeKeyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "id_ed25519.pub")
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

// postedKey returns the JSON body of the POST the registry got.
func postedKey(t *testing.T, reg *httpmock.Registry) map[string]any {
	t.Helper()
	for _, req := range reg.Requests {
		if req.Method == "POST" {
			data, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(data, &body))
			return body
		}
	}
	t.Fatal("no POST request")
	return nil
}

func TestAddRun_Adds(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		title     string
		stdin     bool
		wantTitle string
	}{
		{name: "title from the key's comment", content: pubKey + " dev@laptop\n", wantTitle: "dev@laptop"},
		{name: "--title wins over the comment", content: pubKey + " dev@laptop\n", title: "work laptop", wantTitle: "work laptop"},
		{name: "without a comment and with --title", content: pubKey, title: "ci", wantTitle: "ci"},
		{name: "from standard input", content: pubKey + " piped\n", stdin: true, wantTitle: "piped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/user/keys"), httpmock.StringResponse(http.StatusOK, ownKeysBody))
			reg.Register(httpmock.REST("POST", "/api/v1/user/keys"), httpmock.StringResponse(http.StatusCreated, `{"id":4}`))

			ios, stdin, stdout, stderr := iostreams.Test()
			keyFile := "-"
			if tt.stdin {
				stdin.WriteString(tt.content)
			} else {
				keyFile = writeKeyFile(t, tt.content)
			}
			opts := newOpts(reg, ios, keyFile)
			opts.Title = tt.title

			require.NoError(t, AddRun(opts))
			assert.Equal(t, "Public key added to your account\n", stdout.String())
			assert.Empty(t, stderr.String())
			body := postedKey(t, reg)
			assert.Equal(t, tt.wantTitle, body["title"])
			assert.Equal(t, tt.content, body["key"])
			for _, req := range reg.Requests {
				assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
			}
		})
	}
}

func TestAddRun_AlreadyExists(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user/keys"), httpmock.StringResponse(http.StatusOK,
		`[{"id":3,"title":"other","key":"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQOther"},{"id":5,"title":"mine","key":"`+pubKey+`"}]`))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, AddRun(newOpts(reg, ios, writeKeyFile(t, pubKey+" another-comment\n"))))
	assert.Equal(t, "Public key already exists on your account\n", stdout.String())
	for _, req := range reg.Requests {
		assert.Equal(t, "GET", req.Method)
	}
}

func TestAddRun_RefusedBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "one field", content: "ssh-ed25519\n", wantErr: "provided key is not in a valid format"},
		{name: "empty", content: "", wantErr: "provided key is not in a valid format"},
		{name: "no comment and no --title", content: pubKey + "\n", wantErr: "a title is required: use --title"},
		{
			name:    "a private key",
			content: "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ\n-----END OPENSSH PRIVATE KEY-----\n",
			wantErr: "the key file holds a private key; add the public key (the .pub file)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			ios, _, stdout, _ := iostreams.Test()
			err := AddRun(newOpts(reg, ios, writeKeyFile(t, tt.content)))
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, reg.Requests)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestAddRun_GiteaError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "422 with Gitea's message",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"Key content has been used as non-deploy key","url":"https://app.copia.io/api/swagger"}`,
			wantErr: "failed to add SSH key (HTTP 422): Key content has been used as non-deploy key",
		},
		{
			name:    "a message that quotes the key shows no key content",
			status:  http.StatusUnprocessableEntity,
			body:    `{"message":"Invalid key content: bad key ` + strings.Fields(pubKey)[1] + `"}`,
			wantErr: "failed to add SSH key (HTTP 422): Invalid key content: bad key [key content]",
		},
		{
			name:    "no message",
			status:  http.StatusNotFound,
			body:    ``,
			wantErr: "failed to add SSH key (HTTP 404)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "/api/v1/user/keys"), httpmock.StringResponse(http.StatusOK, `[]`))
			reg.Register(httpmock.REST("POST", "/api/v1/user/keys"), httpmock.StringResponse(tt.status, tt.body))

			ios, _, stdout, _ := iostreams.Test()
			err := AddRun(newOpts(reg, ios, writeKeyFile(t, pubKey+" dev@laptop\n")))
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestAddRun_ListError(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(httpmock.REST("GET", "/api/v1/user/keys"), httpmock.StringResponse(http.StatusForbidden,
		`{"message":"token does not have at least one of required scope(s): [read:user]"}`))

	ios, _, _, _ := iostreams.Test()
	err := AddRun(newOpts(reg, ios, writeKeyFile(t, pubKey+" dev@laptop\n")))
	require.EqualError(t, err, "failed to list SSH keys (HTTP 403): token does not have at least one of required scope(s): [read:user]")
	assert.Len(t, reg.Requests, 1)
}

func TestAddRun_MissingFile(t *testing.T) {
	reg := &httpmock.Registry{}
	ios, _, _, _ := iostreams.Test()
	err := AddRun(newOpts(reg, ios, filepath.Join(t.TempDir(), "missing.pub")))
	require.Error(t, err)
	assert.Empty(t, reg.Requests)
}

func TestNewCmdAdd_Args(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdinTTY bool
		wantFile string
		wantErr  string
	}{
		{name: "a file", args: []string{"key.pub"}, wantFile: "key.pub"},
		{name: "a dash", args: []string{"-"}, stdinTTY: true, wantFile: "-"},
		{name: "no argument with piped stdin", wantFile: "-"},
		{name: "no argument on a terminal", stdinTTY: true, wantErr: "public key file missing"},
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
			cmd.SetArgs(append(tt.args, "--title", "t"))
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFile, got.KeyFile)
			assert.Equal(t, "t", got.Title)
		})
	}
}

func TestNewCmdAdd_Flags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdAdd(&cmdutil.Factory{IOStreams: ios})
	assert.Equal(t, "t", cmd.Flags().Lookup("title").Shorthand)
	assert.Nil(t, cmd.Flags().Lookup("type"))
}
