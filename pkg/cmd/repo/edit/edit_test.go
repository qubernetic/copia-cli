package edit

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

const repoPath = "/api/v1/repos/my-org/my-repo"

// capture answers a request with status and body and stores its JSON
// payload in into.
func capture(t *testing.T, into *map[string]any, status int, body string) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, into))
		return httpmock.StringResponse(status, body)(req)
	}
}

func TestNewCmdEdit_Flags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "description",
			args: []string{"-d", "A repository"},
			want: map[string]any{"description": "A repository"},
		},
		{
			name: "empty description",
			args: []string{"--description", ""},
			want: map[string]any{"description": ""},
		},
		{
			name: "homepage",
			args: []string{"--homepage", "https://example.com"},
			want: map[string]any{"website": "https://example.com"},
		},
		{
			name: "default branch",
			args: []string{"--default-branch", "trunk"},
			want: map[string]any{"default_branch": "trunk"},
		},
		{
			name: "visibility public",
			args: []string{"--visibility", "public", "--accept-visibility-change-consequences"},
			want: map[string]any{"private": false},
		},
		{
			name: "visibility private",
			args: []string{"--visibility", "private", "--accept-visibility-change-consequences"},
			want: map[string]any{"private": true},
		},
		{
			name: "template",
			args: []string{"--template"},
			want: map[string]any{"template": true},
		},
		{
			name: "features",
			args: []string{"--enable-issues", "--enable-wiki=false", "--enable-projects"},
			want: map[string]any{"has_issues": true, "has_wiki": false, "has_projects": true},
		},
		{
			name: "merge styles",
			args: []string{"--enable-merge-commit=false", "--enable-squash-merge", "--enable-rebase-merge=false", "--delete-branch-on-merge"},
			want: map[string]any{
				"allow_merge_commits":               false,
				"allow_squash_merge":                true,
				"allow_rebase":                      false,
				"default_delete_branch_after_merge": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			var payload map[string]any
			reg.Register(httpmock.REST("PATCH", repoPath), capture(t, &payload, http.StatusOK, `{}`))

			ios, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
			cmd := NewCmdEdit(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			require.NoError(t, cmd.Execute())
			assert.Equal(t, tt.want, payload)
			assert.Equal(t, "Edited repository my-org/my-repo\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestEditRun_Topics(t *testing.T) {
	tests := []struct {
		name    string
		current string
		add     []string
		remove  []string
		want    []any
	}{
		{
			name:    "add",
			current: `{"topics":["plc"]}`,
			add:     []string{"scada", " HMI "},
			want:    []any{"plc", "scada", "hmi"},
		},
		{
			name:    "remove",
			current: `{"topics":["plc","scada"]}`,
			remove:  []string{"PLC"},
			want:    []any{"scada"},
		},
		{
			name:    "remove the last one",
			current: `{"topics":["plc"]}`,
			remove:  []string{"plc"},
			want:    []any{},
		},
		{
			name:    "add and remove",
			current: `{"topics":["plc","old"]}`,
			add:     []string{"scada", "plc"},
			remove:  []string{"old"},
			want:    []any{"plc", "scada"},
		},
		{
			name:    "no change sends nothing",
			current: `{"topics":["plc"]}`,
			add:     []string{"plc"},
			remove:  []string{"absent"},
		},
		{
			name:    "no topics yet",
			current: `{"topics":null}`,
			remove:  []string{"absent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", repoPath+"/topics"), httpmock.StringResponse(http.StatusOK, tt.current))
			var payload map[string]any
			if tt.want != nil {
				reg.Register(httpmock.REST("PUT", repoPath+"/topics"), capture(t, &payload, http.StatusNoContent, ``))
			}

			ios, _, stdout, _ := iostreams.Test()
			err := EditRun(&EditOptions{
				IO: ios, HTTPClient: &http.Client{Transport: reg},
				Host: "app.copia.io", Token: "test-token", Owner: "my-org", Repo: "my-repo",
				AddTopics: tt.add, RemoveTopics: tt.remove,
			})
			require.NoError(t, err)
			if tt.want != nil {
				assert.Equal(t, map[string]any{"topics": tt.want}, payload)
			}
			assert.Equal(t, "Edited repository my-org/my-repo\n", stdout.String())
		})
	}
}

func TestNewCmdEdit_SettingsAndTopics(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var settings, topics map[string]any
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/other/repo"), capture(t, &settings, http.StatusOK, `{}`))
	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/repo/topics"), httpmock.StringResponse(http.StatusOK, `{"topics":["old"]}`))
	reg.Register(httpmock.REST("PUT", "/api/v1/repos/other/repo/topics"), capture(t, &topics, http.StatusNoContent, ``))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdEdit(f)
	cmd.SetArgs([]string{"other/repo", "-d", "New", "--add-topic", "plc,scada", "--remove-topic", "old"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, map[string]any{"description": "New"}, settings)
	assert.Equal(t, map[string]any{"topics": []any{"plc", "scada"}}, topics)
	assert.Equal(t, "Edited repository other/repo\n", stdout.String())
}

func TestEditRun_Errors(t *testing.T) {
	desc := "x"
	bad := "internal"

	tests := []struct {
		name      string
		host      string
		opts      EditOptions
		patch     httpmock.Responder
		getTopics httpmock.Responder
		putTopics httpmock.Responder
		wantErr   string
	}{
		{
			name:    "invalid visibility",
			opts:    EditOptions{Visibility: &bad},
			wantErr: `invalid visibility "internal": valid values are {public|private}`,
		},
		{
			name:    "edit refused",
			opts:    EditOptions{Edits: EditRepositoryInput{Description: &desc}},
			patch:   httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: "failed to edit repository (HTTP 403)",
		},
		{
			name: "transport error on edit",
			opts: EditOptions{Edits: EditRepositoryInput{Description: &desc}},
			patch: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:      "topics not readable",
			opts:      EditOptions{AddTopics: []string{"plc"}},
			getTopics: httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr:   "failed to get topics (HTTP 404)",
		},
		{
			name:      "topics not JSON",
			opts:      EditOptions{AddTopics: []string{"plc"}},
			getTopics: httpmock.StringResponse(http.StatusOK, `nope`),
			wantErr:   "parsing topics: ",
		},
		{
			name: "transport error on topics",
			opts: EditOptions{AddTopics: []string{"plc"}},
			getTopics: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name:      "topics refused",
			opts:      EditOptions{AddTopics: []string{"not valid!"}},
			getTopics: httpmock.StringResponse(http.StatusOK, `{"topics":[]}`),
			putTopics: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"invalidTopics":["not valid!"]}`),
			wantErr:   "failed to set topics (HTTP 422)",
		},
		{
			name:    "invalid host on edit",
			host:    "bad\x7fhost",
			opts:    EditOptions{Edits: EditRepositoryInput{Description: &desc}},
			wantErr: "invalid control character",
		},
		{
			name:    "invalid host on topics",
			host:    "bad\x7fhost",
			opts:    EditOptions{AddTopics: []string{"plc"}},
			wantErr: "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.patch != nil {
				reg.Register(httpmock.REST("PATCH", repoPath), tt.patch)
			}
			if tt.getTopics != nil {
				reg.Register(httpmock.REST("GET", repoPath+"/topics"), tt.getTopics)
			}
			if tt.putTopics != nil {
				reg.Register(httpmock.REST("PUT", repoPath+"/topics"), tt.putTopics)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			opts := tt.opts
			opts.IO = ios
			opts.HTTPClient = &http.Client{Transport: reg}
			opts.Host, opts.Token, opts.Owner, opts.Repo = host, "test-token", "my-org", "my-repo"
			err := EditRun(&opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
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
			name:    "no flag",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{},
			wantErr: "specify properties to edit",
		},
		{
			name:    "repository argument only",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"my-org/my-repo"},
			wantErr: "specify properties to edit",
		},
		{
			name:    "visibility without accepting the consequences",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--visibility", "public"},
			wantErr: "use of --visibility flag requires --accept-visibility-change-consequences flag",
		},
		{
			name:    "visibility accepted as false",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--visibility", "private", "--accept-visibility-change-consequences=false"},
			wantErr: "use of --visibility flag requires --accept-visibility-change-consequences flag",
		},
		{
			name:    "invalid visibility",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"--visibility", "internal", "--accept-visibility-change-consequences"},
			wantErr: `invalid visibility "internal": valid values are {public|private}`,
		},
		{
			name:    "too many arguments",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"a/b", "c/d", "-d", "x"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name:    "malformed repository argument",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"just-a-name", "-d", "x"},
			wantErr: "expected owner/repo format",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"-d", "x"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"-d", "x"},
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
