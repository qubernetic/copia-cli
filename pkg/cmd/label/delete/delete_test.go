package delete

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const labelsPath = "/api/v1/repos/my-org/my-repo/labels"

// fullPage answers a page of 50 labels, which makes the pager ask for the
// next one.
func fullPage() string {
	type label struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	labels := make([]label, 50)
	for i := range labels {
		labels[i] = label{ID: int64(i + 1), Name: "x" + string(rune('a'+i%26)) + string(rune('a'+i/26))}
	}
	b, _ := json.Marshal(labels)
	return string(b)
}

func TestDeleteRun_LabelOnSecondPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.QueryMatcher("GET", labelsPath, url.Values{"page": {"1"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, fullPage()),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", labelsPath, url.Values{"page": {"2"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, `[{"id":77,"name":"bug"}]`),
	)
	reg.Register(
		httpmock.REST("DELETE", labelsPath+"/77"),
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
			return httpmock.StringResponse(http.StatusNoContent, "")(req)
		},
	)

	ios, _, stdout, stderr := iostreams.Test()
	err := DeleteRun(&DeleteOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Name:       "bug",
		Confirmed:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, "Label \"bug\" deleted from my-org/my-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDeleteRun_Errors(t *testing.T) {
	list := httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"feature"},{"id":5,"name":"bug"}]`)

	tests := []struct {
		name      string
		host      string
		confirmed bool
		list      httpmock.Responder
		del       httpmock.Responder
		wantErr   string
	}{
		{
			name:    "missing --yes",
			wantErr: `deleting label "bug" from my-org/my-repo is irreversible; use --yes to confirm`,
		},
		{
			name:      "unknown label",
			confirmed: true,
			list:      httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"feature"}]`),
			wantErr:   `label "bug" not found in my-org/my-repo`,
		},
		{
			name:      "label gone before the delete",
			confirmed: true,
			list:      list,
			del:       httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr:   `label "bug" not found in my-org/my-repo`,
		},
		{
			name:      "delete refused",
			confirmed: true,
			list:      list,
			del:       httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr:   "failed to delete label (HTTP 403)",
		},
		{
			name:      "transport error on delete",
			confirmed: true,
			list:      list,
			del: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:      "list fails",
			confirmed: true,
			list:      httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr:   "failed to list labels: API error (HTTP 404)",
		},
		{
			name:      "list is not JSON",
			confirmed: true,
			list:      httpmock.StringResponse(http.StatusOK, `nope`),
			wantErr:   "failed to list labels: parsing response",
		},
		{
			name:      "invalid host",
			confirmed: true,
			host:      "bad\x7fhost",
			wantErr:   "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.list != nil {
				reg.Register(httpmock.REST("GET", labelsPath), tt.list)
			}
			if tt.del != nil {
				reg.Register(httpmock.REST("DELETE", labelsPath+"/5"), tt.del)
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
				Owner:      "my-org",
				Repo:       "my-repo",
				Name:       "bug",
				Confirmed:  tt.confirmed,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdDelete(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	reg.Register(
		httpmock.REST("GET", labelsPath),
		httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"feature"},{"id":5,"name":"bug"}]`),
	)
	reg.Register(httpmock.REST("DELETE", labelsPath+"/5"), httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"bug", "--yes"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Label \"bug\" deleted from my-org/my-repo\n", stdout.String())
}

func TestNewCmdDelete_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name:    "no name argument",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--yes"},
			wantErr: "cannot delete label: name argument required",
		},
		{
			name:    "missing --yes",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"bug"},
			wantErr: "use --yes to confirm",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"bug", "--yes"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"bug", "--yes"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdDelete(tt.factory)
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
