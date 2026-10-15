package upload

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"),
		httpmock.StringResponse(http.StatusOK, `{"id":42,"tag_name":"v1.0.0"}`),
	)
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/releases/42/assets"),
		httpmock.StringResponse(http.StatusCreated, `{"id":1,"name":"binary.tar.gz","size":1024}`),
	)

	// Create temp file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "binary.tar.gz")
	require.NoError(t, os.WriteFile(filePath, []byte("fake binary content"), 0644))

	ios, _, stdout, _ := iostreams.Test()

	opts := &UploadOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
		Files:      []string{filePath},
	}

	err := UploadRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "binary.tar.gz")
}

func TestUploadRun_FileNotFound(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &UploadOptions{
		IO:    ios,
		Tag:   "v1.0.0",
		Files: []string{"/nonexistent/file.tar.gz"},
	}

	err := UploadRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file")
}

const (
	lookupPath = "/api/v1/repos/my-org/my-repo/releases/tags/v1.0.0"
	assetsPath = "/api/v1/repos/my-org/my-repo/releases/42/assets"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func transportError(*http.Request) (*http.Response, error) {
	return nil, errors.New("boom")
}

// drainBody reads the streamed multipart body before answering, so the
// writer goroutine in uploadFile runs to completion.
func drainBody(status int) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		if _, err := io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		return httpmock.StringResponse(status, `{}`)(req)
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	return p
}

func newOpts(reg *httpmock.Registry, files ...string) (*UploadOptions, func() string) {
	ios, _, stdout, _ := iostreams.Test()
	return &UploadOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Tag:        "v1.0.0",
		Files:      files,
	}, stdout.String
}

func TestUploadRun_MultipleFiles(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))

	uploaded := map[string]string{}
	reg.Register(httpmock.REST("POST", assetsPath), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token test-token", req.Header.Get("Authorization"))
		mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)

		mr := multipart.NewReader(req.Body, params["boundary"])
		part, err := mr.NextPart()
		require.NoError(t, err)
		assert.Equal(t, "attachment", part.FormName())
		content, err := io.ReadAll(part)
		require.NoError(t, err)
		uploaded[part.FileName()] = string(content)
		return httpmock.StringResponse(http.StatusCreated, `{}`)(req)
	})

	a := writeTempFile(t, "a.zip", "alpha")
	b := writeTempFile(t, "b.tar.gz", "bravo")
	opts, stdout := newOpts(reg, a, b)

	require.NoError(t, UploadRun(opts))
	assert.Equal(t, map[string]string{"a.zip": "alpha", "b.tar.gz": "bravo"}, uploaded)
	assert.Equal(t, "Uploaded a.zip to v1.0.0\nUploaded b.tar.gz to v1.0.0\n", stdout())
}

func TestUploadRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		stubs   func(reg *httpmock.Registry)
		wantErr string
	}{
		{
			name:    "invalid host",
			host:    "bad\x7fhost",
			wantErr: "invalid control character",
		},
		{
			name: "lookup transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), transportError)
			},
			wantErr: "connecting to app.copia.io: ",
		},
		{
			name: "release not found",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			wantErr: "release v1.0.0 not found",
		},
		{
			name: "lookup body read error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{}), Header: http.Header{}}, nil
				})
			},
			wantErr: "read failed",
		},
		{
			name: "lookup malformed JSON",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{not json`))
			},
			wantErr: "invalid character",
		},
		{
			name: "upload transport error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))
				reg.Register(httpmock.REST("POST", assetsPath), transportError)
			},
			wantErr: "uploading asset.bin: ",
		},
		{
			name: "upload rejected",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))
				reg.Register(httpmock.REST("POST", assetsPath), drainBody(http.StatusRequestEntityTooLarge))
			},
			wantErr: "uploading asset.bin: HTTP 413",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.stubs != nil {
				tt.stubs(reg)
			}

			opts, stdout := newOpts(reg, writeTempFile(t, "asset.bin", "data"))
			if tt.host != "" {
				opts.Host = tt.host
			}

			err := UploadRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout())
		})
	}
}

func TestUploadRun_UnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can open any file")
	}
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))

	p := writeTempFile(t, "secret.bin", "data")
	require.NoError(t, os.Chmod(p, 0))

	opts, stdout := newOpts(reg, p)
	err := UploadRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uploading secret.bin: ")
	assert.Contains(t, err.Error(), "permission denied")
	assert.Empty(t, stdout())
}

func TestUploadRun_CopyError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))
	reg.Register(httpmock.REST("POST", assetsPath), drainBody(http.StatusCreated))

	// A directory passes os.Stat and os.Open but fails on read.
	dir := t.TempDir()
	opts, stdout := newOpts(reg, dir)
	err := UploadRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uploading "+filepath.Base(dir)+": ")
	assert.Empty(t, stdout())
}

func TestNewCmdUpload_RunE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", lookupPath), httpmock.StringResponse(http.StatusOK, `{"id":42}`))
	reg.Register(httpmock.REST("POST", assetsPath), drainBody(http.StatusCreated))

	file := writeTempFile(t, "asset.bin", "data")
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	cmd := NewCmdUpload(f)
	cmd.SetArgs([]string{"v1.0.0", file})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Uploaded asset.bin to v1.0.0\n", stdout.String())
}

func TestNewCmdUpload_RunEErrors(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	tests := []struct {
		name    string
		factory *cmdutil.Factory
		args    []string
		wantErr string
	}{
		{
			name:    "missing files",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"},
			args:    []string{"v1.0.0"},
			wantErr: "requires at least 2 arg(s), only received 1",
		},
		{
			name: "auth failure",
			factory: &cmdutil.Factory{
				Config: func() (*config.Config, error) { return nil, errors.New("no config") },
				Repo:   "my-org/my-repo",
			},
			args:    []string{"v1.0.0", "asset.bin"},
			wantErr: "no config",
		},
		{
			name:    "repo failure",
			factory: &cmdutil.Factory{Host: "app.copia.io", Token: "test-token"},
			args:    []string{"v1.0.0", "asset.bin"},
			wantErr: "could not determine repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			tt.factory.IOStreams = ios
			cmd := NewCmdUpload(tt.factory)
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
