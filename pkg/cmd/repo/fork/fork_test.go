package fork

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	forksPath   = "/api/v1/repos/upstream-org/project/forks"
	johnFork    = `{"name":"project","full_name":"john/project","html_url":"https://app.copia.io/john/project","default_branch":"main","owner":{"login":"john"}}`
	orgFork     = `{"name":"project","full_name":"my-org/project","html_url":"https://app.copia.io/my-org/project","default_branch":"main","owner":{"login":"my-org"}}`
	conflictMsg = `{"message":"repository is already forked by user"}`
)

// decodeInto returns a responder that decodes the request body into v and
// then answers with respond.
func decodeInto(v any, respond httpmock.Responder) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(v); err != nil {
			return nil, err
		}
		return respond(req)
	}
}

func TestForkRun_API(t *testing.T) {
	tests := []struct {
		name       string
		opts       ForkOptions
		stubs      func(reg *httpmock.Registry)
		wantSent   forkRequest
		wantStdout string
		wantStderr string
		wantBody   map[string]string // body of the PATCH that renames
	}{
		{
			name: "new fork",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, johnFork))
			},
			wantStdout: "https://app.copia.io/john/project\n",
		},
		{
			name: "new fork in an organization under another name",
			opts: ForkOptions{Org: "my-org", ForkName: "renamed"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted,
					`{"name":"renamed","full_name":"my-org/renamed","html_url":"https://app.copia.io/my-org/renamed","owner":{"login":"my-org"}}`))
			},
			wantSent:   forkRequest{Organization: "my-org", Name: "renamed"},
			wantStdout: "https://app.copia.io/my-org/renamed\n",
		},
		{
			name: "existing fork of the user",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"John"}`))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+orgFork+","+johnFork+"]"))
			},
			wantStderr: "john/project already exists\n",
		},
		{
			name: "existing fork in an organization",
			opts: ForkOptions{Org: "my-org"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+johnFork+","+orgFork+"]"))
			},
			wantSent:   forkRequest{Organization: "my-org"},
			wantStderr: "my-org/project already exists\n",
		},
		{
			name: "existing fork under another name is renamed",
			opts: ForkOptions{Org: "my-org", ForkName: "renamed"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+orgFork+"]"))
				reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/project"), httpmock.StringResponse(http.StatusOK, `{}`))
			},
			wantSent:   forkRequest{Organization: "my-org", Name: "renamed"},
			wantStderr: "my-org/project already exists\n",
			wantBody:   map[string]string{"name": "renamed"},
		},
		{
			name: "existing fork with the same name in another case is kept",
			opts: ForkOptions{Org: "my-org", ForkName: "Project"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+orgFork+"]"))
			},
			wantSent:   forkRequest{Organization: "my-org", Name: "Project"},
			wantStderr: "my-org/project already exists\n",
		},
		{
			name: "default branch only deletes the other branches of a new fork",
			opts: ForkOptions{DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, johnFork))
				reg.Register(httpmock.REST("GET", "/api/v1/repos/john/project/branches"),
					httpmock.StringResponse(http.StatusOK, `[{"name":"main"},{"name":"dev"},{"name":"feature/x y"}]`))
				reg.Register(httpmock.REST("DELETE", "/api/v1/repos/john/project/branches/dev"), httpmock.StringResponse(http.StatusNoContent, ""))
				reg.Register(httpmock.REST("DELETE", "/api/v1/repos/john/project/branches/feature/x y"), httpmock.StringResponse(http.StatusNoContent, ""))
			},
			wantStdout: "https://app.copia.io/john/project\n",
		},
		{
			name: "default branch only leaves an existing fork alone",
			opts: ForkOptions{Org: "my-org", DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+orgFork+"]"))
			},
			wantSent:   forkRequest{Organization: "my-org"},
			wantStderr: "my-org/project already exists\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)

			ios, _, stdout, stderr := iostreams.Test()
			opts := tt.opts
			opts.IO = ios
			opts.HTTPClient = &http.Client{Transport: reg}
			opts.Host, opts.Token = "app.copia.io", "test-token"
			opts.Owner, opts.Repo = "upstream-org", "project"

			require.NoError(t, ForkRun(&opts))
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())

			var sent forkRequest
			require.NoError(t, json.NewDecoder(reg.Requests[0].Body).Decode(&sent))
			assert.Equal(t, tt.wantSent, sent)
			for _, req := range reg.Requests {
				assert.Equal(t, "token test-token", req.Header.Get("Authorization"), req.URL.Path)
				if req.Method == "PATCH" {
					var got map[string]string
					require.NoError(t, json.NewDecoder(req.Body).Decode(&got))
					assert.Equal(t, tt.wantBody, got)
				}
			}
		})
	}
}

func TestForkRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		opts    ForkOptions
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name: "not found",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusNotFound, `{"message":"not found"}`))
			},
			wantErr: "failed to fork repository (HTTP 404): {\"message\":\"not found\"}",
		},
		{
			name: "conflict without a fork",
			opts: ForkOptions{Org: "my-org"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, `{"message":"repository already exists"}`))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+johnFork+"]"))
			},
			wantErr: "failed to fork repository (HTTP 409): {\"message\":\"repository already exists\"}",
		},
		{
			name: "user lookup fails",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusUnauthorized, `{}`))
			},
			wantErr: "failed to get the authenticated user (HTTP 401)",
		},
		{
			name: "user lookup transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
			},
			wantErr: "connecting to app.copia.io",
		},
		{
			name: "malformed user",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{not json`))
			},
			wantErr: "invalid character",
		},
		{
			name: "fork list fails",
			opts: ForkOptions{Org: "my-org"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "failed to list forks: API error (HTTP 403)",
		},
		{
			name: "malformed fork list",
			opts: ForkOptions{Org: "my-org"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, `{not json`))
			},
			wantErr: "failed to list forks",
		},
		{
			name: "rename fails",
			opts: ForkOptions{Org: "my-org", ForkName: "renamed"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusConflict, conflictMsg))
				reg.Register(httpmock.REST("GET", forksPath), httpmock.StringResponse(http.StatusOK, "["+orgFork+"]"))
				reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/project"), httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"name taken"}`))
			},
			wantErr: "could not rename fork: failed to rename repository (HTTP 422): name taken",
		},
		{
			name: "branch list fails",
			opts: ForkOptions{DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, johnFork))
				reg.Register(httpmock.REST("GET", "/api/v1/repos/john/project/branches"), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "could not delete the other branches of the fork: API error (HTTP 404)",
		},
		{
			name: "malformed branch list",
			opts: ForkOptions{DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, johnFork))
				reg.Register(httpmock.REST("GET", "/api/v1/repos/john/project/branches"), httpmock.StringResponse(http.StatusOK, `{not json`))
			},
			wantErr: "could not delete the other branches of the fork",
		},
		{
			name: "default branch unknown",
			opts: ForkOptions{DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted,
					`{"name":"project","full_name":"john/project","owner":{"login":"john"}}`))
			},
			wantErr: "could not delete the other branches of the fork: the fork has no default branch",
		},
		{
			name: "branch delete fails",
			opts: ForkOptions{DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, johnFork))
				reg.Register(httpmock.REST("GET", "/api/v1/repos/john/project/branches"), httpmock.StringResponse(http.StatusOK, `[{"name":"main"},{"name":"dev"}]`))
				reg.Register(httpmock.REST("DELETE", "/api/v1/repos/john/project/branches/dev"), httpmock.StringResponse(http.StatusForbidden, `{}`))
			},
			wantErr: "could not delete the other branches of the fork: deleting branch dev (HTTP 403)",
		},
		{
			name: "branch delete transport error",
			opts: ForkOptions{DefaultBranchOnly: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, johnFork))
				reg.Register(httpmock.REST("GET", "/api/v1/repos/john/project/branches"), httpmock.StringResponse(http.StatusOK, `[{"name":"dev"}]`))
			},
			wantErr: "connecting to app.copia.io",
		},
		{
			name: "malformed json",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), httpmock.StringResponse(http.StatusAccepted, `{not json`))
			},
			wantErr: "invalid character",
		},
		{
			name: "body read error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", forksPath), func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(errReader{})}, nil
				})
			},
			wantErr: "read failed",
		},
		{
			name:    "transport error",
			stubs:   func(*httpmock.Registry) {},
			wantErr: "connecting to app.copia.io",
		},
		{
			name:    "invalid host",
			host:    "bad host",
			stubs:   func(*httpmock.Registry) {},
			wantErr: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, _, _ := iostreams.Test()
			opts := tt.opts
			opts.IO = ios
			opts.HTTPClient = &http.Client{Transport: reg}
			opts.Host, opts.Token = host, "test-token"
			opts.Owner, opts.Repo = "upstream-org", "project"

			err := ForkRun(&opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
