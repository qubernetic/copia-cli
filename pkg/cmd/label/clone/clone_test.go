package clone

import (
	"encoding/json"
	"errors"
	"fmt"
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

const (
	sourcePath = "/api/v1/repos/src-org/src-repo/labels"
	destPath   = "/api/v1/repos/my-org/my-repo/labels"
)

const sourceLabels = `[
	{"id":1,"name":"bug","color":"e11d48","description":"Something is broken"},
	{"id":2,"name":"docs","color":"0969da","description":""}
]`

// recordJSON answers status and appends the decoded request body to into.
func recordJSON(t *testing.T, into *[]map[string]any, status int) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		*into = append(*into, payload)
		return httpmock.StringResponse(status, `{}`)(req)
	}
}

func cloneOpts(reg *httpmock.Registry, ios *iostreams.IOStreams, force bool) *CloneOptions {
	return &CloneOptions{
		IO:          ios,
		HTTPClient:  &http.Client{Transport: reg},
		Host:        "app.copia.io",
		Token:       "test-token",
		Owner:       "my-org",
		Repo:        "my-repo",
		SourceOwner: "src-org",
		SourceRepo:  "src-repo",
		Force:       force,
	}
}

func TestCloneRun_NewLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", sourcePath), httpmock.StringResponse(http.StatusOK, sourceLabels))
	reg.Register(httpmock.REST("GET", destPath), httpmock.StringResponse(http.StatusOK, `[]`))
	var created []map[string]any
	reg.Register(httpmock.REST("POST", destPath), recordJSON(t, &created, http.StatusCreated))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, CloneRun(cloneOpts(reg, ios, false)))

	assert.Equal(t, []map[string]any{
		{"name": "bug", "color": "e11d48", "description": "Something is broken"},
		{"name": "docs", "color": "0969da", "description": ""},
	}, created)
	assert.Equal(t, "Cloned 2 labels from src-org/src-repo to my-org/my-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestCloneRun_SkipsExistingLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", sourcePath), httpmock.StringResponse(http.StatusOK, sourceLabels))
	reg.Register(httpmock.REST("GET", destPath), httpmock.StringResponse(http.StatusOK,
		`[{"id":5,"name":"bug","color":"000000","description":"Ours"},{"id":6,"name":"keep","color":"ffffff"}]`))
	var created []map[string]any
	reg.Register(httpmock.REST("POST", destPath), recordJSON(t, &created, http.StatusCreated))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, CloneRun(cloneOpts(reg, ios, false)))

	assert.Equal(t, []map[string]any{{"name": "docs", "color": "0969da", "description": ""}}, created)
	for _, req := range reg.Requests {
		assert.NotEqual(t, "PATCH", req.Method, "an existing label was changed without --force")
		assert.NotEqual(t, "DELETE", req.Method, "a destination label was deleted")
	}
	assert.Empty(t, stdout.String())
	assert.Equal(t, "! Cloned 1 label of 2 from src-org/src-repo to my-org/my-repo\n", stderr.String())
}

func TestCloneRun_ForceOverwritesExistingLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", sourcePath), httpmock.StringResponse(http.StatusOK, sourceLabels))
	reg.Register(httpmock.REST("GET", destPath), httpmock.StringResponse(http.StatusOK,
		`[{"id":5,"name":"bug","color":"000000","description":"Ours"},{"id":6,"name":"docs","color":"ffffff","description":"Ours too"}]`))
	var updatedBug, updatedDocs []map[string]any
	reg.Register(httpmock.REST("PATCH", destPath+"/5"), recordJSON(t, &updatedBug, http.StatusOK))
	reg.Register(httpmock.REST("PATCH", destPath+"/6"), recordJSON(t, &updatedDocs, http.StatusOK))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, CloneRun(cloneOpts(reg, ios, true)))

	assert.Equal(t, []map[string]any{{"color": "e11d48", "description": "Something is broken"}}, updatedBug)
	assert.Equal(t, []map[string]any{{"color": "0969da", "description": ""}}, updatedDocs, "--force clears a description the source has not")
	assert.Equal(t, "Cloned 2 labels from src-org/src-repo to my-org/my-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestCloneRun_ReadsEveryPage(t *testing.T) {
	page := make([]map[string]any, 50)
	for i := range page {
		page[i] = map[string]any{"id": i + 1, "name": fmt.Sprintf("label-%02d", i), "color": "111111"}
	}
	full, _ := json.Marshal(page)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.QueryMatcher("GET", sourcePath, url.Values{"page": {"1"}}), httpmock.StringResponse(http.StatusOK, string(full)))
	reg.Register(httpmock.QueryMatcher("GET", sourcePath, url.Values{"page": {"2"}}), httpmock.StringResponse(http.StatusOK, `[{"id":51,"name":"last","color":"222222"}]`))
	reg.Register(httpmock.QueryMatcher("GET", destPath, url.Values{"page": {"1"}}), httpmock.StringResponse(http.StatusOK, string(full)))
	reg.Register(httpmock.QueryMatcher("GET", destPath, url.Values{"page": {"2"}}), httpmock.StringResponse(http.StatusOK, `[]`))
	var created []map[string]any
	reg.Register(httpmock.REST("POST", destPath), recordJSON(t, &created, http.StatusCreated))

	ios, _, _, stderr := iostreams.Test()
	require.NoError(t, CloneRun(cloneOpts(reg, ios, false)))

	assert.Equal(t, []map[string]any{{"name": "last", "color": "222222", "description": ""}}, created)
	assert.Equal(t, "! Cloned 1 label of 51 from src-org/src-repo to my-org/my-repo\n", stderr.String())
}

func TestCloneRun_EmptySource(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", sourcePath), httpmock.StringResponse(http.StatusOK, `[]`))
	reg.Register(httpmock.REST("GET", destPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, CloneRun(cloneOpts(reg, ios, false)))
	assert.Equal(t, "Cloned 0 labels from src-org/src-repo to my-org/my-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestCloneRun_Errors(t *testing.T) {
	dest := `[{"id":5,"name":"bug"}]`

	tests := []struct {
		name    string
		host    string
		source  httpmock.Responder
		dest    httpmock.Responder
		create  httpmock.Responder
		update  httpmock.Responder
		wantErr string
	}{
		{
			name:    "source list fails",
			source:  httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "failed to list labels of src-org/src-repo: API error (HTTP 404)",
		},
		{
			name:    "source list is not JSON",
			source:  httpmock.StringResponse(http.StatusOK, `nope`),
			wantErr: "failed to list labels of src-org/src-repo: parsing response",
		},
		{
			name:    "destination list fails",
			source:  httpmock.StringResponse(http.StatusOK, sourceLabels),
			dest:    httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: "failed to list labels of my-org/my-repo: API error (HTTP 403)",
		},
		{
			name:    "create refused",
			source:  httpmock.StringResponse(http.StatusOK, sourceLabels),
			dest:    httpmock.StringResponse(http.StatusOK, dest),
			create:  httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"invalid color"}`),
			update:  httpmock.StringResponse(http.StatusOK, `{}`),
			wantErr: `failed to create label "docs": HTTP 422`,
		},
		{
			name:    "update refused",
			source:  httpmock.StringResponse(http.StatusOK, sourceLabels),
			dest:    httpmock.StringResponse(http.StatusOK, dest),
			update:  httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: `failed to update label "bug": HTTP 403`,
		},
		{
			name:   "transport error on create",
			source: httpmock.StringResponse(http.StatusOK, sourceLabels),
			dest:   httpmock.StringResponse(http.StatusOK, dest),
			create: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			update:  httpmock.StringResponse(http.StatusOK, `{}`),
			wantErr: `failed to create label "docs": connecting to app.copia.io: `,
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
			if tt.source != nil {
				reg.Register(httpmock.REST("GET", sourcePath), tt.source)
			}
			if tt.dest != nil {
				reg.Register(httpmock.REST("GET", destPath), tt.dest)
			}
			if tt.create != nil {
				reg.Register(httpmock.REST("POST", destPath), tt.create)
			}
			if tt.update != nil {
				reg.Register(httpmock.REST("PATCH", destPath+"/5"), tt.update)
			}

			ios, _, stdout, stderr := iostreams.Test()
			opts := cloneOpts(reg, ios, true)
			if tt.host != "" {
				opts.Host = tt.host
			}
			err := CloneRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestCloneRun_SourceErrorSendsNoWrite(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(httpmock.REST("GET", sourcePath), httpmock.StringResponse(http.StatusInternalServerError, `{}`))

	ios, _, _, _ := iostreams.Test()
	require.Error(t, CloneRun(cloneOpts(reg, ios, true)))
	require.Len(t, reg.Requests, 1, "a failed source list stops before the destination is read or written")
}

func TestCloneRun_EscapesRepositoryNames(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(func(req *http.Request) bool {
		return req.URL.EscapedPath() == "/api/v1/repos/src-org/a%2F..%2Fb/labels"
	}, httpmock.StringResponse(http.StatusOK, `[]`))
	reg.Register(httpmock.REST("GET", destPath), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, _, _ := iostreams.Test()
	opts := cloneOpts(reg, ios, false)
	opts.SourceRepo = "a/../b"
	require.NoError(t, CloneRun(opts))
	assert.Equal(t, "/api/v1/repos/src-org/a%2F..%2Fb/labels", reg.Requests[0].URL.EscapedPath())
}

func TestNewCmdClone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", sourcePath), httpmock.StringResponse(http.StatusOK, sourceLabels))
	reg.Register(httpmock.REST("GET", destPath), httpmock.StringResponse(http.StatusOK, `[{"id":5,"name":"bug"},{"id":6,"name":"docs"}]`))
	reg.Register(httpmock.REST("PATCH", destPath+"/5"), httpmock.StringResponse(http.StatusOK, `{}`))
	reg.Register(httpmock.REST("PATCH", destPath+"/6"), httpmock.StringResponse(http.StatusOK, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdClone(f)
	cmd.SetArgs([]string{"src-org/src-repo", "-f"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Cloned 2 labels from src-org/src-repo to my-org/my-repo\n", stdout.String())
}

func TestNewCmdClone_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name:    "no source argument",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--force"},
			wantErr: "cannot clone labels: source-repository argument required",
		},
		{
			name:    "source is not owner/repo",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"src-repo"},
			wantErr: `expected owner/repo format, got "src-repo"`,
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"src-org/src-repo"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"src-org/src-repo"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdClone(tt.factory)
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
