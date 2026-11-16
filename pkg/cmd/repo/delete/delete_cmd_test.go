package delete

import (
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

func runDeleteCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdDelete(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdDelete_Run(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo"),
		httpmock.StringResponse(http.StatusNoContent, ``))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	require.NoError(t, runDeleteCmd(f, "my-org/my-repo", "--yes"))
	assert.Equal(t, "Deleted repository my-org/my-repo\n", stdout.String())
}

func TestNewCmdDelete_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory func(ios *iostreams.IOStreams) *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name: "auth failure",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{
					IOStreams: ios,
					Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
				}
			},
			args:    []string{"my-org/my-repo", "--yes"},
			wantErr: "no config",
		},
		{
			name: "invalid repo",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			args:    []string{"my-repo", "--yes"},
			wantErr: "expected owner/repo format",
		},
		{
			name: "missing arg",
			factory: func(ios *iostreams.IOStreams) *cmdutil.Factory {
				return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			},
			wantErr: "accepts 1 arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			err := runDeleteCmd(tt.factory(ios), tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDeleteRun_RequestErrors(t *testing.T) {
	t.Run("transport error", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo"),
			func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") })

		ios, _, _, _ := iostreams.Test()
		err := DeleteRun(&DeleteOptions{
			IO: ios, HTTPClient: &http.Client{Transport: reg},
			Host: "app.copia.io", Token: "test-token",
			Owner: "my-org", Repo: "my-repo", Confirmed: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connecting to app.copia.io")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("invalid host", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		err := DeleteRun(&DeleteOptions{
			IO: ios, HTTPClient: &http.Client{},
			Host: "bad host", Token: "test-token",
			Owner: "my-org", Repo: "my-repo", Confirmed: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})
}
