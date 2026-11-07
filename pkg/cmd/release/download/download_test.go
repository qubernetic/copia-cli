package download

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	assetBase   = "https://app.copia.io/my-org/my-repo/releases/download/v1.0.0/"
	releaseJSON = `{"id":42,"tag_name":"v1.0.0","assets":[` +
		`{"id":4,"name":"a.zip","browser_download_url":"` + assetBase + `a.zip"},` +
		`{"id":5,"name":"b.tar.gz","browser_download_url":"` + assetBase + `b.tar.gz"}]}`
)

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams) *DownloadOptions {
	return &DownloadOptions{
		IO:          ios,
		HTTPClient:  &http.Client{Transport: reg},
		Host:        "app.copia.io",
		Token:       "test-token",
		Owner:       "my-org",
		Repo:        "my-repo",
		Tag:         "v1.0.0",
		Destination: ".",
	}
}

func assetResponder(t *testing.T, body string) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(http.StatusOK, body)(req)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestDownloadRun_Pattern(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
	reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/b.tar.gz"), assetResponder(t, "tarball"))

	dir := t.TempDir()
	ios, _, stdout, stderr := iostreams.Test()
	opts := newOpts(reg, ios)
	opts.Destination = dir
	opts.FilePatterns = []string{"*.tar.gz"}

	require.NoError(t, DownloadRun(opts))
	assert.Equal(t, "tarball", readFile(t, filepath.Join(dir, "b.tar.gz")))
	assert.NoFileExists(t, filepath.Join(dir, "a.zip"))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDownloadRun_AllAssetsLatest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases"),
		httpmock.StringResponse(http.StatusOK, "["+releaseJSON+"]"),
	)
	reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/a.zip"), assetResponder(t, "zip"))
	reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/b.tar.gz"), assetResponder(t, "tarball"))

	dir := filepath.Join(t.TempDir(), "new", "dir")
	ios, _, stdout, _ := iostreams.Test()
	opts := newOpts(reg, ios)
	opts.Tag = ""
	opts.Destination = dir

	require.NoError(t, DownloadRun(opts))
	assert.Equal(t, "zip", readFile(t, filepath.Join(dir, "a.zip")))
	assert.Equal(t, "tarball", readFile(t, filepath.Join(dir, "b.tar.gz")))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no temporary file left behind")
	assert.Empty(t, stdout.String())
}

func TestDownloadRun_Archive(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		disposition string
		wantFile    string
	}{
		{name: "zip named by the server", format: "zip", disposition: `attachment; filename="my-repo-v1.0.0.zip"`, wantFile: "my-repo-v1.0.0.zip"},
		{name: "tar.gz without a name", format: "tar.gz", wantFile: "my-repo-v1.0.0.tar.gz"},
		{name: "server name with a path", format: "zip", disposition: `attachment; filename="../evil.zip"`, wantFile: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
			archive := assetResponder(t, "archive")
			if tt.disposition != "" {
				archive = httpmock.WithHeader(archive, "Content-Disposition", tt.disposition)
			}
			reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/archive/v1.0.0."+tt.format), archive)

			dir := t.TempDir()
			ios, _, _, _ := iostreams.Test()
			opts := newOpts(reg, ios)
			opts.Destination = dir
			opts.ArchiveType = tt.format

			err := DownloadRun(opts)
			if tt.wantFile == "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), `refusing to write "../evil.zip"`)
				assert.NoFileExists(t, filepath.Join(filepath.Dir(dir), "evil.zip"))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "archive", readFile(t, filepath.Join(dir, tt.wantFile)))
		})
	}
}

func TestDownloadRun_Output(t *testing.T) {
	t.Run("to a file", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
		reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/a.zip"), assetResponder(t, "zip"))

		out := filepath.Join(t.TempDir(), "renamed.bin")
		ios, _, stdout, _ := iostreams.Test()
		opts := newOpts(reg, ios)
		opts.FilePatterns = []string{"*.zip"}
		opts.OutputFile = out

		require.NoError(t, DownloadRun(opts))
		assert.Equal(t, "zip", readFile(t, out))
		assert.Empty(t, stdout.String())
	})

	t.Run("to standard output", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
		reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/b.tar.gz"), assetResponder(t, "tarball"))

		ios, _, stdout, _ := iostreams.Test()
		opts := newOpts(reg, ios)
		opts.FilePatterns = []string{"b.*"}
		opts.OutputFile = "-"

		require.NoError(t, DownloadRun(opts))
		assert.Equal(t, "tarball", stdout.String())
	})

	t.Run("more than one asset", func(t *testing.T) {
		reg := &httpmock.Registry{}
		reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))

		ios, _, _, _ := iostreams.Test()
		opts := newOpts(reg, ios)
		opts.OutputFile = "-"

		err := DownloadRun(opts)
		require.EqualError(t, err, "unable to write more than one asset with `--output`, got 2 assets")
		assert.Len(t, reg.Requests, 1)
	})
}

func TestDownloadRun_ExistingFile(t *testing.T) {
	tests := []struct {
		name      string
		clobber   bool
		skip      bool
		toFile    bool
		wantErr   string
		wantAfter string
	}{
		{name: "fails without a flag", wantErr: "a.zip already exists (use `--clobber` to overwrite file or `--skip-existing` to skip file)", wantAfter: "old"},
		{name: "--output fails without a flag", toFile: true, wantErr: "already exists", wantAfter: "old"},
		{name: "--clobber overwrites", clobber: true, wantAfter: "zip"},
		{name: "--clobber overwrites --output", clobber: true, toFile: true, wantAfter: "zip"},
		{name: "--skip-existing skips", skip: true, wantAfter: "old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
			reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/a.zip"), assetResponder(t, "zip"))

			dir := t.TempDir()
			existing := filepath.Join(dir, "a.zip")
			require.NoError(t, os.WriteFile(existing, []byte("old"), 0o600))

			ios, _, _, _ := iostreams.Test()
			opts := newOpts(reg, ios)
			opts.FilePatterns = []string{"a.zip"}
			opts.OverwriteExisting = tt.clobber
			opts.SkipExisting = tt.skip
			if tt.toFile {
				opts.OutputFile = existing
			} else {
				opts.Destination = dir
			}

			err := DownloadRun(opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantAfter, readFile(t, existing))
			if tt.wantAfter == "old" {
				assert.Len(t, reg.Requests, 1, "the asset is not requested")
			}
		})
	}
}

func TestDownloadRun_UnsafeAssetNames(t *testing.T) {
	for _, name := range []string{"../evil", "..", ".", "sub/evil", `sub\evil`, "/etc/evil", "a..b", ""} {
		t.Run(name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST("GET", tagPath),
				httpmock.StringResponse(http.StatusOK, `{"id":42,"tag_name":"v1.0.0","assets":[`+
					`{"id":4,"name":`+strconv.Quote(name)+`,"browser_download_url":"`+assetBase+`x"}]}`),
			)

			parent := t.TempDir()
			dir := filepath.Join(parent, "dir")
			ios, _, _, _ := iostreams.Test()
			opts := newOpts(reg, ios)
			opts.Destination = dir

			err := DownloadRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "an asset name must be a plain file name")
			assert.Len(t, reg.Requests, 1, "the asset is not requested")
			entries, err := os.ReadDir(parent)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestDownloadRun_OtherHost(t *testing.T) {
	t.Run("asset URL on another host", func(t *testing.T) {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", tagPath),
			httpmock.StringResponse(http.StatusOK, `{"id":42,"tag_name":"v1.0.0","assets":[`+
				`{"id":4,"name":"a.zip","browser_download_url":"https://evil.example/a.zip"},`+
				`{"id":5,"name":"b.zip","browser_download_url":"http://app.copia.io/b.zip"}]}`),
		)

		dir := t.TempDir()
		for _, pattern := range []string{"a.zip", "b.zip"} {
			ios, _, _, _ := iostreams.Test()
			opts := newOpts(reg, ios)
			opts.Destination = dir
			opts.FilePatterns = []string{pattern}

			err := DownloadRun(opts)
			require.EqualError(t, err, "refusing to download "+pattern+": its URL is not on app.copia.io")
		}
		assert.Len(t, reg.Requests, 2, "only the release lookups")
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("redirect to another host", func(t *testing.T) {
		reg := &httpmock.Registry{}
		httpmock.ReplaceDefaultTransport(t, reg)
		reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
		reg.Register(
			httpmock.REST("GET", "/releases/download/v1.0.0/a.zip"),
			httpmock.WithHeader(httpmock.StringResponse(http.StatusFound, ""), "Location", "https://storage.example/a.zip?X-Amz-Signature=secret"),
		)

		dir := t.TempDir()
		ios, _, _, _ := iostreams.Test()
		opts := newOpts(reg, ios)
		opts.HTTPClient = cmdutil.NewHTTPClient()
		opts.Destination = dir
		opts.FilePatterns = []string{"a.zip"}

		err := DownloadRun(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing redirect from https://app.copia.io to https://storage.example")
		assert.NotContains(t, err.Error(), "X-Amz-Signature", "a signed redirect URL is not printed")
		assert.NotContains(t, err.Error(), "/a.zip", "no URL is printed, only hosts")
		for _, req := range reg.Requests {
			assert.Equal(t, "app.copia.io", req.URL.Host, "nothing is sent to another host")
		}
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestDownloadRun_Errors(t *testing.T) {
	release := httpmock.StringResponse(http.StatusOK, releaseJSON)

	tests := []struct {
		name     string
		host     string
		patterns []string
		archive  string
		release  httpmock.Responder
		asset    httpmock.Responder
		wantErr  string
	}{
		{name: "invalid host", host: "bad/host", wantErr: `invalid host "bad/host"`},
		{name: "unknown release", release: httpmock.StringResponse(http.StatusNotFound, `{}`), wantErr: "release v1.0.0 not found"},
		{name: "no match", patterns: []string{"*.deb"}, release: release, wantErr: "no assets match the file pattern"},
		{
			name:    "no assets",
			release: httpmock.StringResponse(http.StatusOK, `{"id":42,"tag_name":"v1.0.0","assets":[]}`),
			wantErr: "no assets to download",
		},
		{
			name:    "archive of a release without a tag",
			archive: "zip",
			release: httpmock.StringResponse(http.StatusOK, `{"id":42,"name":"Draft","tag_name":""}`),
			wantErr: `release "Draft" has no tag, so it has no "zip" archive`,
		},
		{
			name:     "asset refused",
			patterns: []string{"a.zip"},
			release:  release,
			asset:    httpmock.StringResponse(http.StatusNotFound, ``),
			wantErr:  "failed to download a.zip (HTTP 404)",
		},
		{
			name:    "archive refused",
			archive: "zip",
			release: release,
			asset:   httpmock.StringResponse(http.StatusForbidden, ``),
			wantErr: "failed to download the source archive (HTTP 403)",
		},
		{
			name:     "transport error",
			patterns: []string{"a.zip"},
			release:  release,
			asset:    func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr:  "downloading a.zip: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.release != nil {
				reg.Register(httpmock.REST("GET", tagPath), tt.release)
			}
			switch {
			case tt.asset != nil && tt.archive != "":
				reg.Register(httpmock.REST("GET", "/archive/v1.0.0.zip"), tt.asset)
			case tt.asset != nil:
				reg.Register(httpmock.REST("GET", "/a.zip"), tt.asset)
			}

			dir := t.TempDir()
			ios, _, stdout, _ := iostreams.Test()
			opts := newOpts(reg, ios)
			if tt.host != "" {
				opts.Host = tt.host
			}
			opts.Destination = dir
			opts.FilePatterns = tt.patterns
			opts.ArchiveType = tt.archive

			err := DownloadRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestNewCmdDownload(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", tagPath), httpmock.StringResponse(http.StatusOK, releaseJSON))
	reg.Register(httpmock.REST("GET", "/releases/download/v1.0.0/a.zip"), assetResponder(t, "zip"))

	dir := t.TempDir()
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdDownload(f)
	cmd.SetArgs([]string{"v1.0.0", "-p", "*.zip", "-D", dir})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "zip", readFile(t, filepath.Join(dir, "a.zip")))
	assert.Empty(t, stdout.String())
}

func TestNewCmdDownload_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")
	valid := func() *cmdutil.Factory {
		return &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	}

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{name: "too many arguments", factory: valid(), args: []string{"v1", "v2"}, wantErr: "accepts at most 1 arg(s), received 2"},
		{name: "latest without --pattern or --archive", factory: valid(), args: nil, wantErr: "`--pattern` or `--archive` is required when downloading the latest release"},
		{name: "--clobber and --skip-existing", factory: valid(), args: []string{"v1", "--clobber", "--skip-existing"}, wantErr: "specify only one of `--clobber` or `--skip-existing`"},
		{name: "--dir and --output", factory: valid(), args: []string{"v1", "-D", "x", "-O", "y"}, wantErr: "specify only one of `--dir` or `--output`"},
		{name: "--pattern and --archive", factory: valid(), args: []string{"-p", "*", "-A", "zip"}, wantErr: "specify only one of '--pattern' or '--archive'"},
		{name: "unknown archive format", factory: valid(), args: []string{"-A", "rar"}, wantErr: "the value for `--archive` must be one of \"zip\" or \"tar.gz\""},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"v1"},
			wantErr: "no config",
		},
		{name: "repo failure", factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"}, args: []string{"v1"}, wantErr: "could not determine repository"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdDownload(tt.factory)
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
