package merge

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runMerge(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdMerge(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdMerge_RunE(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantDelete bool
		wantStdout string
	}{
		{name: "default merge", args: []string{"7"}, wantMethod: "merge", wantStdout: "Merged PR #7 (merge)\n"},
		{name: "explicit merge", args: []string{"7", "--merge"}, wantMethod: "merge", wantStdout: "Merged PR #7 (merge)\n"},
		{name: "squash", args: []string{"7", "--squash"}, wantMethod: "squash", wantStdout: "Merged PR #7 (squash)\n"},
		{
			name:       "rebase with delete",
			args:       []string{"7", "--rebase", "--delete-branch"},
			wantMethod: "rebase",
			wantDelete: true,
			wantStdout: "Merged PR #7 (rebase)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
				httpmock.StringResponse(http.StatusOK, ``))

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

			require.NoError(t, runMerge(t, f, tt.args...))
			assert.Equal(t, tt.wantStdout, stdout.String())

			require.Len(t, reg.Requests, 1)
			var sent mergeRequest
			require.NoError(t, json.NewDecoder(reg.Requests[0].Body).Decode(&sent))
			assert.Equal(t, tt.wantMethod, sent.Do)
			assert.Equal(t, tt.wantDelete, sent.DeleteBranchAfterMerge)
		})
	}
}

func TestNewCmdMerge_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name: "invalid number",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios}
			},
			args:    []string{"abc"},
			wantErr: "invalid PR number: abc",
		},
		{
			name: "auth failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) {
					return nil, errors.New("no config")
				}}
			},
			args:    []string{"7"},
			wantErr: "no config",
		},
		{
			name: "repo failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			args:    []string{"7"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runMerge(t, tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMergeRun_Errors(t *testing.T) {
	const path = "/api/v1/repos/my-org/my-repo/pulls/7/merge"

	tests := []struct {
		name      string
		host      string
		responder httpmock.Responder
		wantErr   string
	}{
		{
			name:    "bad url",
			host:    "bad host",
			wantErr: "invalid character",
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:      "not mergeable",
			responder: httpmock.StringResponse(http.StatusMethodNotAllowed, `{}`),
			wantErr:   "failed to merge PR (HTTP 405)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("POST", path), tt.responder)
			}

			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}
			ios, _, stdout, _ := iostreams.Test()
			err := MergeRun(&MergeOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Number:     7,
				Method:     "merge",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
