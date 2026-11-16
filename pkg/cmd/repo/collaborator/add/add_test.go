package add

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
	cmd := NewCmdAdd(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// added answers 204 to a PUT that carries the token and wantBody.
func added(t *testing.T, wantBody string) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.JSONEq(t, wantBody, string(body))
		return httpmock.StringResponse(http.StatusNoContent, "")(req)
	}
}

func TestNewCmdAdd(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		permission string
	}{
		{name: "default", args: []string{"alice"}, permission: "write"},
		{name: "read", args: []string{"alice", "--permission", "read"}, permission: "read"},
		{name: "write", args: []string{"alice", "-p", "write"}, permission: "write"},
		{name: "admin", args: []string{"alice", "-p", "admin"}, permission: "admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("PUT", alicePath), added(t, `{"permission":"`+tt.permission+`"}`))

			ios, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

			require.NoError(t, runCmd(f, tt.args...))
			assert.Equal(t, "Added alice to my-org/my-repo with "+tt.permission+" permission\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestNewCmdAdd_Refused(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no login", args: []string{}, wantErr: "accepts 1 arg(s), received 0"},
		{name: "two logins", args: []string{"alice", "bob"}, wantErr: "accepts 1 arg(s), received 2"},
		{name: "owner permission", args: []string{"alice", "-p", "owner"}, wantErr: `invalid permission "owner": use read, write or admin`},
		{name: "upper case permission", args: []string{"alice", "-p", "Write"}, wantErr: `invalid permission "Write": use read, write or admin`},
		{name: "empty permission", args: []string{"alice", "-p", ""}, wantErr: `invalid permission "": use read, write or admin`},
		{name: "login with a slash", args: []string{"a/b"}, wantErr: `invalid login "a/b"`},
		{name: "dot dot", args: []string{".."}, wantErr: `invalid login ".."`},
		{name: "empty login", args: []string{""}, wantErr: `invalid login ""`},
		{name: "login with a control character", args: []string{"al\x1bice"}, wantErr: `invalid login "al\x1bice"`},
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

func TestAddRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "unknown user",
			respond: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"user redirect does not exist [name: alice]"}`),
			wantErr: "user alice not found",
		},
		{
			name:    "unknown user without a message",
			respond: httpmock.StringResponse(http.StatusUnprocessableEntity, ``),
			wantErr: "user alice not found",
		},
		{
			name:    "other unprocessable entity",
			respond: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"user is inactive"}`),
			wantErr: "failed to add collaborator (HTTP 422): user is inactive",
		},
		{
			name:    "missing scope",
			respond: httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [write:repository]"}`),
			wantErr: "failed to add collaborator (HTTP 403): token does not have at least one of required scope(s): [write:repository]",
		},
		{
			name:    "no message",
			respond: httpmock.StringResponse(http.StatusInternalServerError, `oops`),
			wantErr: "failed to add collaborator (HTTP 500)",
		},
		{
			name:    "transport error",
			respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "connecting to app.copia.io",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("PUT", alicePath), tt.respond)

			ios, _, stdout, _ := iostreams.Test()
			err := AddRun(&AddOptions{
				IO: ios, HTTPClient: &http.Client{Transport: reg},
				Host: "app.copia.io", Token: "test-token",
				Owner: "my-org", Repo: "my-repo",
				Login: "alice", Permission: "write",
			})
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
