package view

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/cmdutil/browsertest"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tagPath    = "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"
	latestPath = "/api/v1/repos/my-org/my-repo/releases/latest"
	releaseURL = "https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0"
)

const releaseJSON = `{
	"id": 42,
	"tag_name": "v1.0.0",
	"name": "First release",
	"body": "Release notes",
	"draft": false,
	"prerelease": false,
	"created_at": "2026-01-02T03:04:05Z",
	"published_at": "2026-01-03T03:04:05Z",
	"author": {"id": 7, "login": "alice", "email": "alice@example.com"},
	"html_url": "https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0",
	"tarball_url": "https://app.copia.io/my-org/my-repo/archive/v1.0.0.tar.gz",
	"zipball_url": "https://app.copia.io/my-org/my-repo/archive/v1.0.0.zip",
	"target_commitish": "main",
	"assets": [
		{"id": 5, "name": "plc.zip", "size": 2048, "download_count": 3, "browser_download_url": "https://app.copia.io/attachments/a", "uuid": "a"},
		{"id": 6, "name": "notes.txt", "size": 12, "download_count": 0, "browser_download_url": "https://app.copia.io/attachments/b", "uuid": "b"}
	]
}`

const plainOutput = "title:\tFirst release\n" +
	"tag:\tv1.0.0\n" +
	"draft:\tfalse\n" +
	"prerelease:\tfalse\n" +
	"author:\talice\n" +
	"created:\t2026-01-02T03:04:05Z\n" +
	"published:\t2026-01-03T03:04:05Z\n" +
	"url:\t" + releaseURL + "\n" +
	"asset:\tplc.zip\n" +
	"asset:\tnotes.txt\n" +
	"--\n" +
	"Release notes\n"

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdView(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdView_Tag(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))

	ios, _, stdout, stderr := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "v1.0.0"))
	assert.Equal(t, plainOutput, stdout.String())
	assert.Empty(t, stderr.String())
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
}

func TestNewCmdView_Latest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", latestPath), httpmock.StringResponse(http.StatusOK, releaseJSON))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios)))
	assert.Equal(t, plainOutput, stdout.String())
}

func TestNewCmdView_NotFound(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		path    string
		wantErr string
	}{
		{name: "tag", args: []string{"v1.0.0"}, path: tagPath, wantErr: "release v1.0.0 not found"},
		{name: "latest", path: latestPath, wantErr: "release not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST("GET", tt.path), httpmock.StringResponse(http.StatusNotFound, `{}`))

			ios, _, stdout, _ := iostreams.Test()
			require.EqualError(t, runCmd(t, readyFactory(ios), tt.args...), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestNewCmdView_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))

	ios, _, stdout, _ := iostreams.Test()
	fields := "id,tag_name,name,body,draft,prerelease,created_at,published_at,author,html_url,tarball_url,zipball_url,target_commitish,assets"
	require.NoError(t, runCmd(t, readyFactory(ios), "v1.0.0", "--json", fields))
	assert.JSONEq(t, `{
		"id": 42,
		"tag_name": "v1.0.0",
		"name": "First release",
		"body": "Release notes",
		"draft": false,
		"prerelease": false,
		"created_at": "2026-01-02T03:04:05Z",
		"published_at": "2026-01-03T03:04:05Z",
		"author": {"login": "alice"},
		"html_url": "https://app.copia.io/my-org/my-repo/releases/tag/v1.0.0",
		"tarball_url": "https://app.copia.io/my-org/my-repo/archive/v1.0.0.tar.gz",
		"zipball_url": "https://app.copia.io/my-org/my-repo/archive/v1.0.0.zip",
		"target_commitish": "main",
		"assets": [
			{"id": 5, "name": "plc.zip", "size": 2048, "download_count": 3, "browser_download_url": "https://app.copia.io/attachments/a"},
			{"id": 6, "name": "notes.txt", "size": 12, "download_count": 0, "browser_download_url": "https://app.copia.io/attachments/b"}
		]
	}`, stdout.String())
}

func TestNewCmdView_JQ(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", latestPath), httpmock.StringResponse(http.StatusOK, releaseJSON))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, readyFactory(ios), "--json", "assets", "--jq", ".assets[].name"))
	assert.Equal(t, "plc.zip\nnotes.txt\n", stdout.String())
}

func TestNewCmdView_InvalidJSONField(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, readyFactory(ios), "--json", "url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown JSON field: "url"`)
	assert.Empty(t, reg.Requests)
}

func ttyOpts(t *testing.T, body string) (*ViewOptions, func() string) {
	t.Helper()
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, body))

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	return &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
		Now:        func() time.Time { return time.Date(2026, 1, 5, 3, 4, 5, 0, time.UTC) },
	}, stdout.String
}

func TestViewRun_TTY(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "release with assets",
			body: releaseJSON,
			want: "v1.0.0\n" +
				"alice released this 2 days ago\n" +
				"\n" +
				"Release notes\n" +
				"\n" +
				"Assets\n" +
				"NAME       SIZE\n" +
				"plc.zip    2.00 KiB\n" +
				"notes.txt  12 B\n" +
				"\n" +
				"View on Copia: " + releaseURL + "\n",
		},
		{
			name: "draft without body or assets",
			body: `{"tag_name":"v2.0.0","draft":true,"created_at":"2026-01-05T01:04:05Z","author":{"login":"bob"},"html_url":"u"}`,
			want: "v2.0.0\n" +
				"Draft • bob created this 2 hours ago\n" +
				"\n" +
				"View on Copia: u\n",
		},
		{
			name: "pre-release with unparsable time",
			body: `{"tag_name":"v3.0.0","prerelease":true,"published_at":"soon","author":{"login":"eve"},"html_url":"u"}`,
			want: "v3.0.0\n" +
				"Pre-release • eve released this soon\n" +
				"\n" +
				"View on Copia: u\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, stdout := ttyOpts(t, tt.body)
			require.NoError(t, ViewRun(opts))
			assert.Equal(t, tt.want, stdout())
		})
	}
}

func TestViewRun_PlainDraft(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK,
		`{"tag_name":"v1.0.0","name":"n","draft":true,"created_at":"c","published_at":"p","author":{"login":"bob"},"html_url":"u","body":"ends with newline\n"}`))

	ios, _, stdout, _ := iostreams.Test()
	opts := &ViewOptions{IO: ios, HTTPClient: &http.Client{Transport: reg}, Host: "app.copia.io", Token: "t", Owner: "my-org", Repo: "my-repo", Tag: "v1.0.0"}
	require.NoError(t, ViewRun(opts))
	assert.Equal(t, "title:\tn\ntag:\tv1.0.0\ndraft:\ttrue\nprerelease:\tfalse\nauthor:\tbob\ncreated:\tc\nurl:\tu\n--\nends with newline\n", stdout.String())
}

func TestViewRun_StartsPager(t *testing.T) {
	opts, _ := ttyOpts(t, releaseJSON)
	ios, _, out, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetPager("copia-no-such-pager")
	opts.IO = ios

	require.NoError(t, ViewRun(opts))
	assert.Contains(t, stderr.String(), "failed to start pager: ")
	assert.Contains(t, out.String(), "View on Copia: ")
}

func TestHumanFileSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.00 KiB"},
		{1535, "1.49 KiB"},
		{5 * 1024 * 1024, "5.00 MiB"},
		{3 * 1024 * 1024 * 1024, "3.00 GiB"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, humanFileSize(tt.size))
	}
}

func TestNewCmdView_Web(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		repo    string
		tty     bool
		wantURL string
		wantErr string
	}{
		{name: "tag", args: []string{"v1.0.0", "--web"}, wantURL: releaseURL},
		{name: "latest", args: []string{"-w"}, wantURL: "https://app.copia.io/my-org/my-repo/releases/latest"},
		{name: "escaped tag", args: []string{"release/1.0", "-w"}, wantURL: "https://app.copia.io/my-org/my-repo/releases/tag/release%2F1.0"},
		{name: "repo flag", args: []string{"v1.0.0", "-w"}, repo: "other-org/other-repo", wantURL: "https://app.copia.io/other-org/other-repo/releases/tag/v1.0.0"},
		{
			name: "tty", args: []string{"v1.0.0", "-w"}, tty: true,
			wantURL: releaseURL,
			wantErr: "Opening app.copia.io/my-org/my-repo/releases/tag/v1.0.0 in your browser.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPIA_TOKEN", "")
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			b := &browsertest.Browser{}
			repo := tt.repo
			if repo == "" {
				repo = "my-org/my-repo"
			}
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: repo, Browser: b}

			require.NoError(t, runCmd(t, f, tt.args...))
			assert.Equal(t, []string{tt.wantURL}, b.URLs)
			assert.Empty(t, stdout.String())
			assert.Equal(t, tt.wantErr, stderr.String())
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestNewCmdView_WebErrors(t *testing.T) {
	t.Setenv("COPIA_HOST", "")
	ios, _, _, _ := iostreams.Test()
	noConfig := &cmdutil.Factory{IOStreams: ios, Repo: "my-org/my-repo", Browser: &browsertest.Browser{},
		Config: func() (*config.Config, error) { return nil, errors.New("no config") }}
	require.EqualError(t, runCmd(t, noConfig, "-w"), "no config")

	noRepo := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Browser: &browsertest.Browser{}}
	err := runCmd(t, noRepo, "-w")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not determine repository")

	failing := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Repo: "my-org/my-repo",
		Browser: &browsertest.Browser{Err: errors.New("no browser")}}
	require.EqualError(t, runCmd(t, failing, "-w"), "no browser")
}

func TestNewCmdView_Errors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name:    "too many args",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"v1", "v2"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"v1.0.0"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"v1.0.0"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			err := runCmd(t, tt.factory, tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestViewRun_FetchError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusInternalServerError, `{}`))

	ios, _, stdout, _ := iostreams.Test()
	opts := &ViewOptions{IO: ios, HTTPClient: &http.Client{Transport: reg}, Host: "app.copia.io", Token: "t", Owner: "my-org", Repo: "my-repo", Tag: "v1.0.0"}
	require.EqualError(t, ViewRun(opts), "API error (HTTP 500)")
	assert.Empty(t, stdout.String())
}
