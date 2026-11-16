package deleteasset

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

const (
	tagPath     = "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"
	releaseJSON = `{"id":42,"tag_name":"v1.0.0","assets":[{"id":4,"name":"a.zip"},{"id":5,"name":"b.zip"}]}`
)

func TestDeleteAssetRun(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
	reg.Register(
		httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo/releases/42/assets/5"),
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
			return httpmock.StringResponse(http.StatusNoContent, "")(req)
		},
	)

	ios, _, stdout, stderr := iostreams.Test()
	err := DeleteAssetRun(&DeleteAssetOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
		AssetName:  "b.zip",
		Confirmed:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, "Deleted asset b.zip from release v1.0.0\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDeleteAssetRun_Errors(t *testing.T) {
	release := httpmock.StringResponse(http.StatusOK, releaseJSON)

	tests := []struct {
		name      string
		host      string
		confirmed bool
		asset     string
		release   httpmock.Responder
		del       httpmock.Responder
		wantErr   string
	}{
		{
			name:    "missing --yes",
			asset:   "b.zip",
			wantErr: "deleting asset b.zip from release v1.0.0 is irreversible; use --yes to confirm",
		},
		{
			name:      "unknown release",
			confirmed: true,
			asset:     "b.zip",
			release:   httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr:   "release v1.0.0 not found",
		},
		{
			name:      "unknown asset",
			confirmed: true,
			asset:     "c.zip",
			release:   release,
			wantErr:   "asset c.zip not found in release v1.0.0",
		},
		{
			name:      "asset gone before the delete",
			confirmed: true,
			asset:     "b.zip",
			release:   release,
			del:       httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr:   "asset b.zip not found in release v1.0.0",
		},
		{
			name:      "delete refused",
			confirmed: true,
			asset:     "b.zip",
			release:   release,
			del:       httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr:   "failed to delete asset (HTTP 403)",
		},
		{
			name:      "transport error on delete",
			confirmed: true,
			asset:     "b.zip",
			release:   release,
			del: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:      "invalid host",
			confirmed: true,
			asset:     "b.zip",
			host:      "bad\x7fhost",
			wantErr:   "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.release != nil {
				reg.Register(httpmock.REST("GET", tagPath), tt.release)
			}
			if tt.del != nil {
				reg.Register(httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo/releases/42/assets/5"), tt.del)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := DeleteAssetRun(&DeleteAssetOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Tag:        "v1.0.0",
				AssetName:  tt.asset,
				Confirmed:  tt.confirmed,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdDeleteAsset(t *testing.T) {
	for _, yes := range []string{"--yes", "-y"} {
		t.Run(yes, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
			reg.Register(
				httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo/releases/42/assets/4"),
				httpmock.StringResponse(http.StatusNoContent, ""),
			)

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			cmd := NewCmdDeleteAsset(f)
			cmd.SetArgs([]string{"v1.0.0", "a.zip", yes})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			require.NoError(t, cmd.Execute())
			assert.Equal(t, "Deleted asset a.zip from release v1.0.0\n", stdout.String())
		})
	}
}

func TestNewCmdDeleteAsset_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name:    "no asset argument",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"v1.0.0", "--yes"},
			wantErr: "accepts 2 arg(s), received 1",
		},
		{
			name:    "missing --yes",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"v1.0.0", "a.zip"},
			wantErr: "use --yes to confirm",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"v1.0.0", "a.zip", "--yes"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"v1.0.0", "a.zip", "--yes"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdDeleteAsset(tt.factory)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}
