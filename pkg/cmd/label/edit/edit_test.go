package edit

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

// labelPage answers one page of the label list; a full page of 50 makes the
// pager ask for the next one.
func labelPage(names ...string) string {
	type label struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	labels := make([]label, 0, len(names))
	for i, n := range names {
		labels = append(labels, label{ID: int64(i + 1), Name: n})
	}
	b, _ := json.Marshal(labels)
	return string(b)
}

func fullPage(prefix string) []string {
	names := make([]string, 50)
	for i := range names {
		names[i] = prefix + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	return names
}

func TestEditRun_LabelOnSecondPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.QueryMatcher("GET", labelsPath, url.Values{"page": {"1"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, labelPage(fullPage("x")...)),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", labelsPath, url.Values{"page": {"2"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, `[{"id":77,"name":"bug"}]`),
	)
	var payload map[string]any
	reg.Register(
		httpmock.REST("PATCH", labelsPath+"/77"),
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &payload))
			return httpmock.StringResponse(http.StatusOK, `{"id":77,"name":"bug"}`)(req)
		},
	)

	ios, _, stdout, stderr := iostreams.Test()
	err := EditRun(&EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Name:       "bug",
		Color:      "ff0000",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"color": "ff0000"}, payload)
	assert.Equal(t, "Label \"bug\" updated in my-org/my-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNewCmdEdit_Flags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "color without hash",
			args: []string{"bug", "--color", "ff0000"},
			want: map[string]any{"color": "ff0000"},
		},
		{
			name: "color with leading hash",
			args: []string{"bug", "-c", "#0969da"},
			want: map[string]any{"color": "0969da"},
		},
		{
			name: "description",
			args: []string{"bug", "-d", "Something is broken"},
			want: map[string]any{"description": "Something is broken"},
		},
		{
			name: "new name",
			args: []string{"bug", "-n", "big-bug"},
			want: map[string]any{"name": "big-bug"},
		},
		{
			name: "every flag",
			args: []string{"bug", "--name", "big-bug", "--color", "#e11d48", "--description", "Bigger than normal bug"},
			want: map[string]any{"name": "big-bug", "color": "e11d48", "description": "Bigger than normal bug"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			reg.Register(
				httpmock.REST("GET", labelsPath),
				httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"feature"},{"id":5,"name":"bug"}]`),
			)
			var payload map[string]any
			reg.Register(
				httpmock.REST("PATCH", labelsPath+"/5"),
				func(req *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal(body, &payload))
					return httpmock.StringResponse(http.StatusOK, `{"id":5}`)(req)
				},
			)

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			cmd := NewCmdEdit(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			require.NoError(t, cmd.Execute())
			assert.Equal(t, tt.want, payload)
			assert.Equal(t, "Label \"bug\" updated in my-org/my-repo\n", stdout.String())
		})
	}
}

func TestEditRun_Errors(t *testing.T) {
	list := httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"feature"},{"id":5,"name":"bug"}]`)

	tests := []struct {
		name    string
		host    string
		newName string
		list    httpmock.Responder
		patch   httpmock.Responder
		wantErr string
	}{
		{
			name:    "unknown label",
			list:    httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"feature"}]`),
			wantErr: `label "bug" not found in my-org/my-repo`,
		},
		{
			name:    "name taken by another label",
			newName: "feature",
			list:    list,
			wantErr: `label with name "feature" already exists`,
		},
		{
			name:    "name conflict reported by the API",
			newName: "big-bug",
			list:    list,
			patch:   httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"label exists"}`),
			wantErr: `label with name "big-bug" already exists`,
		},
		{
			name:    "list fails",
			list:    httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "failed to list labels: API error (HTTP 404)",
		},
		{
			name:    "list is not JSON",
			list:    httpmock.StringResponse(http.StatusOK, `nope`),
			wantErr: "failed to list labels: parsing response",
		},
		{
			name: "transport error on update",
			list: list,
			patch: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:    "update refused",
			list:    list,
			patch:   httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"invalid color"}`),
			wantErr: "failed to update label (HTTP 422)",
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
				reg.Register(httpmock.REST("GET", labelsPath), tt.list)
			}
			if tt.patch != nil {
				reg.Register(httpmock.REST("PATCH", labelsPath+"/5"), tt.patch)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := EditRun(&EditOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Name:       "bug",
				NewName:    tt.newName,
				Color:      "zzz",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestEditRun_RenameToOwnName(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", labelsPath),
		httpmock.StringResponse(http.StatusOK, `[{"id":5,"name":"bug"}]`),
	)
	reg.Register(httpmock.REST("PATCH", labelsPath+"/5"), httpmock.StringResponse(http.StatusOK, `{"id":5}`))

	ios, _, stdout, _ := iostreams.Test()
	err := EditRun(&EditOptions{
		IO: ios, HTTPClient: &http.Client{Transport: reg},
		Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo",
		Name: "bug", NewName: "bug",
	})
	require.NoError(t, err)
	assert.Equal(t, "Label \"bug\" updated in my-org/my-repo\n", stdout.String())
}

func TestNewCmdEdit_RunEErrors(t *testing.T) {
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
			args:    []string{"--color", "000000"},
			wantErr: "cannot update label: name argument required",
		},
		{
			name:    "no flag",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"bug"},
			wantErr: "specify at least one of --color, --description, or --name",
		},
		{
			name:    "only a hash as color",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"bug", "--color", "#"},
			wantErr: "specify at least one of --color, --description, or --name",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"bug", "--color", "000000"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"bug", "--color", "000000"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdEdit(tt.factory)
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
