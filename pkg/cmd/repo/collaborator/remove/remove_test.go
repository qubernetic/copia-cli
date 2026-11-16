package remove

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const alicePath = "/api/v1/repos/my-org/my-repo/collaborators/alice"

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdRemove(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// noContent answers 204 to a request that carries the token.
func noContent(t *testing.T) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(http.StatusNoContent, "")(req)
	}
}

func TestNewCmdRemove(t *testing.T) {
	for _, yes := range []string{"--yes", "-y"} {
		t.Run(yes, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", alicePath), noContent(t))
			reg.Register(httpmock.REST("DELETE", alicePath), noContent(t))

			ios, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

			require.NoError(t, runCmd(f, "alice", yes))
			assert.Equal(t, "Removed alice from my-org/my-repo\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestNewCmdRemove_Refused(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no --yes", args: []string{"alice"}, wantErr: "--yes required when not running interactively"},
		{name: "no --yes before the login check", args: []string{"a/b"}, wantErr: "--yes required when not running interactively"},
		{name: "no login", args: []string{"--yes"}, wantErr: "accepts 1 arg(s), received 0"},
		{name: "two logins", args: []string{"alice", "bob", "--yes"}, wantErr: "accepts 1 arg(s), received 2"},
		{name: "login with a slash", args: []string{"a/b", "--yes"}, wantErr: `invalid login "a/b"`},
		{name: "dot dot", args: []string{"..", "--yes"}, wantErr: `invalid login ".."`},
		{name: "empty login", args: []string{"", "--yes"}, wantErr: `invalid login ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

			require.EqualError(t, runCmd(f, tt.args...), tt.wantErr)
			assert.Empty(t, stdout.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestRemoveRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		login   string
		host    string
		check   httpmock.Responder
		del     httpmock.Responder
		wantErr string
	}{
		{
			name:    "not a collaborator",
			check:   httpmock.StringResponse(http.StatusNotFound, ``),
			wantErr: "alice is not a collaborator of my-org/my-repo",
		},
		{
			name:    "check refused with a message",
			check:   httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [read:repository]"}`),
			wantErr: "failed to check collaborator (HTTP 403): token does not have at least one of required scope(s): [read:repository]",
		},
		{
			name:    "check refused without a message",
			check:   httpmock.StringResponse(http.StatusInternalServerError, `oops`),
			wantErr: "failed to check collaborator (HTTP 500)",
		},
		{
			name:    "transport error on check",
			check:   func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "connecting to app.copia.io",
		},
		{
			name:    "delete refused with a message",
			check:   httpmock.StringResponse(http.StatusNoContent, ``),
			del:     httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [write:repository]"}`),
			wantErr: "failed to remove collaborator (HTTP 403): token does not have at least one of required scope(s): [write:repository]",
		},
		{
			name:    "delete refused without a message",
			check:   httpmock.StringResponse(http.StatusNoContent, ``),
			del:     httpmock.StringResponse(http.StatusInternalServerError, ``),
			wantErr: "failed to remove collaborator (HTTP 500)",
		},
		{
			name:    "transport error on delete",
			check:   httpmock.StringResponse(http.StatusNoContent, ``),
			del:     func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "connecting to app.copia.io",
		},
		{
			name:    "invalid host",
			host:    "bad\x7fhost",
			wantErr: "invalid control character",
		},
		{
			name:    "invalid login",
			login:   "a/b",
			wantErr: `invalid login "a/b"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.check != nil {
				reg.Register(httpmock.REST("GET", alicePath), tt.check)
			}
			if tt.del != nil {
				reg.Register(httpmock.REST("DELETE", alicePath), tt.del)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			login := tt.login
			if login == "" {
				login = "alice"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := RemoveRun(&RemoveOptions{
				IO: ios, HTTPClient: &http.Client{Transport: reg},
				Host: host, Token: "test-token",
				Owner: "my-org", Repo: "my-repo",
				Login: login,
			})
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
