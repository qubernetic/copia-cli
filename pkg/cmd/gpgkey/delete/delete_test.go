package delete

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const keysPath = "/api/v1/user/gpg_keys"

// fullPage answers a page of 50 keys, which makes the pager ask for the
// next one.
func fullPage() string {
	type key struct {
		ID    int64  `json:"id"`
		KeyID string `json:"key_id"`
	}
	keys := make([]key, 50)
	for i := range keys {
		keys[i] = key{ID: int64(i + 1), KeyID: fmt.Sprintf("00000000000000%02X", i)}
	}
	b, _ := json.Marshal(keys)
	return string(b)
}

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdDelete(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func deleted(t *testing.T) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(http.StatusNoContent, "")(req)
	}
}

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name  string
		keyID string
		page2 string
	}{
		{name: "match on the second page", keyID: "3AA5C34371567BD2", page2: `[{"id":77,"key_id":"3AA5C34371567BD2"}]`},
		{name: "match in another case", keyID: "3aa5c34371567bd2", page2: `[{"id":77,"key_id":"3AA5C34371567BD2"}]`},
	}

	for _, tt := range tests {
		for _, yes := range []string{"--yes", "-y"} {
			t.Run(tt.name+" "+yes, func(t *testing.T) {
				reg := &httpmock.Registry{}
				defer reg.Verify(t)
				httpmock.ReplaceDefaultTransport(t, reg)
				reg.Register(
					httpmock.QueryMatcher("GET", keysPath, url.Values{"page": {"1"}, "limit": {"50"}}),
					httpmock.StringResponse(http.StatusOK, fullPage()),
				)
				reg.Register(
					httpmock.QueryMatcher("GET", keysPath, url.Values{"page": {"2"}, "limit": {"50"}}),
					httpmock.StringResponse(http.StatusOK, tt.page2),
				)
				reg.Register(httpmock.REST("DELETE", keysPath+"/77"), deleted(t))

				ios, _, stdout, stderr := iostreams.Test()
				f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

				require.NoError(t, runCmd(f, tt.keyID, yes))
				assert.Equal(t, "GPG key "+tt.keyID+" deleted from your account\n", stdout.String())
				assert.Empty(t, stderr.String())
			})
		}
	}
}

func TestNewCmdDelete_Refused(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no --yes", args: []string{"3AA5C34371567BD2"}, wantErr: "--yes required when not running interactively"},
		{name: "no key ID", args: []string{"--yes"}, wantErr: "cannot delete: key id required"},
		{name: "two key IDs", args: []string{"A", "B", "--yes"}, wantErr: "cannot delete: key id required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

			require.EqualError(t, runCmd(f, tt.args...), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestDeleteRun_Errors(t *testing.T) {
	list := httpmock.StringResponse(http.StatusOK, `[{"id":3,"key_id":"1111111111111111"},{"id":5,"key_id":"3AA5C34371567BD2"}]`)

	tests := []struct {
		name    string
		host    string
		list    httpmock.Responder
		del     httpmock.Responder
		wantErr string
	}{
		{
			name:    "not found",
			list:    httpmock.StringResponse(http.StatusOK, `[{"id":3,"key_id":"1111111111111111"}]`),
			wantErr: "unable to delete GPG key 3AA5C34371567BD2: either the GPG key is not found or it is not owned by you",
		},
		{
			name:    "no keys",
			list:    httpmock.StringResponse(http.StatusOK, `[]`),
			wantErr: "unable to delete GPG key 3AA5C34371567BD2: either the GPG key is not found or it is not owned by you",
		},
		{
			name:    "list fails with a message",
			list:    httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [read:user]"}`),
			wantErr: "failed to list GPG keys (HTTP 403): token does not have at least one of required scope(s): [read:user]",
		},
		{
			name:    "list is not JSON",
			list:    httpmock.StringResponse(http.StatusOK, `nope`),
			wantErr: "parsing response",
		},
		{
			name:    "delete refused with a message",
			list:    list,
			del:     httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [write:user]"}`),
			wantErr: "failed to delete GPG key (HTTP 403): token does not have at least one of required scope(s): [write:user]",
		},
		{
			name:    "delete refused without a message",
			list:    list,
			del:     httpmock.StringResponse(http.StatusInternalServerError, ``),
			wantErr: "failed to delete GPG key (HTTP 500)",
		},
		{
			name: "transport error on delete",
			list: list,
			del: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:    "invalid host",
			host:    "bad\x7fhost",
			wantErr: "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.list != nil {
				reg.Register(httpmock.REST("GET", keysPath), tt.list)
			}
			if tt.del != nil {
				reg.Register(httpmock.REST("DELETE", keysPath+"/5"), tt.del)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := DeleteRun(&DeleteOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				KeyID:      "3AA5C34371567BD2",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
