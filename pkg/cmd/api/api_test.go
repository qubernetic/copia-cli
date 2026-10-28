package api

import (
	"encoding/json"
	"errors"
	"io"
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

func TestApiRun_GET(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/user"),
		httpmock.StringResponse(http.StatusOK, `{"login":"john","id":1}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Method:     "GET",
		Path:       "/user",
	}

	err := APIRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "john")
}

func TestApiRun_POST_WithFields(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues"),
		httpmock.StringResponse(http.StatusCreated, `{"number":1,"title":"test issue"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Method:     "POST",
		Path:       "/repos/my-org/my-repo/issues",
		RawFields:  []string{"title=test issue", "body=description"},
	}

	err := APIRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "test issue")
}

func TestApiRun_DELETE(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("DELETE", "/api/v1/repos/my-org/my-repo"),
		httpmock.StringResponse(http.StatusNoContent, ``),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Method:     "DELETE",
		Path:       "/repos/my-org/my-repo",
	}

	err := APIRun(opts)
	require.NoError(t, err)
}

func TestApiRun_DefaultMethodGET(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/version"),
		httpmock.StringResponse(http.StatusOK, `{"version":"1.21.0"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Path:       "/version",
	}

	err := APIRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "1.21.0")
}

func TestApiRun_MissingPath(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &APIOptions{IO: ios, Path: ""}

	err := APIRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path required")
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errBody) Close() error             { return nil }

func runAPICmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdAPI(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdAPI_FlagsAndBody(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var gotBody map[string]string
	var gotReq *http.Request
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo/issues/3"),
		func(req *http.Request) (*http.Response, error) {
			gotReq = req
			b, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(b, &gotBody))
			return httpmock.StringResponse(http.StatusOK, `{"number":3}`)(req)
		})

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "repos/my-org/my-repo/issues/3",
		"-X", "PATCH", "-f", "title=new=title", "-f", "ignored", "-H", "X-Custom:  value ", "-H", "nocolon")
	require.NoError(t, err)

	assert.Equal(t, "{\n  \"number\": 3\n}\n", stdout.String())
	assert.Equal(t, map[string]string{"title": "new=title"}, gotBody)
	require.NotNil(t, gotReq)
	assert.Equal(t, "app.copia.io", gotReq.URL.Host)
	assert.Equal(t, "token test-token", gotReq.Header.Get("Authorization"))
	assert.Equal(t, "application/json", gotReq.Header.Get("Content-Type"))
	assert.Equal(t, "value", gotReq.Header.Get("X-Custom"))
	assert.Empty(t, gotReq.Header.Get("nocolon"))
}

func TestNewCmdAPI_FieldDefaultsToPOST(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusCreated, `{"id":1}`))

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "/api/v1/repos/my-org/my-repo/labels", "--field", "name=bug")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"id": 1`)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "/api/v1/repos/my-org/my-repo/labels", reg.Requests[0].URL.Path)
}

func TestNewCmdAPI_AuthFailure(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runAPICmd(t, f, "/user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}

func TestNewCmdAPI_RequiresOneArg(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	err := runAPICmd(t, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg(s)")
}

func TestApiRun_Errors(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		host      string
		responder httpmock.Responder
		wantErr   string
		wantOut   string
	}{
		{
			name:    "invalid method",
			method:  "BAD METHOD",
			wantErr: "invalid method",
		},
		{
			name:    "invalid host",
			host:    "bad host",
			wantErr: "invalid character",
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io",
		},
		{
			name: "body read error",
			responder: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: errBody{}}, nil
			},
			wantErr: "read failed",
		},
		{
			name:      "http error with json body",
			responder: httpmock.StringResponse(http.StatusNotFound, `{"message":"not found"}`),
			wantErr:   "API error (HTTP 404)",
			wantOut:   "{\n  \"message\": \"not found\"\n}\n",
		},
		{
			name:      "http error with plain body",
			responder: httpmock.StringResponse(http.StatusInternalServerError, `oops`),
			wantErr:   "API error (HTTP 500)",
			wantOut:   "oops\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.responder != nil {
				reg.Register(httpmock.REST("GET", "/api/v1/user"), tt.responder)
			}
			host := tt.host
			if host == "" {
				host = "app.copia.io"
			}

			ios, _, stdout, _ := iostreams.Test()
			err := APIRun(&APIOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       host,
				Token:      "test-token",
				Method:     tt.method,
				Path:       "user",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}

func TestApiRun_NonJSONSuccess(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/o/r/raw/README.md"),
		httpmock.StringResponse(http.StatusOK, "# Title"))

	ios, _, stdout, _ := iostreams.Test()
	err := APIRun(&APIOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Path:       "/repos/o/r/raw/README.md",
	})
	require.NoError(t, err)
	assert.Equal(t, "# Title\n", stdout.String())
	assert.Empty(t, reg.Requests[0].Header.Get("Content-Type"))
}

func captureBody(t *testing.T, reg *httpmock.Registry, method, path string, got *map[string]any) {
	t.Helper()
	reg.Register(httpmock.REST(method, path),
		func(req *http.Request) (*http.Response, error) {
			b, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(b, got))
			return httpmock.StringResponse(http.StatusCreated, `{}`)(req)
		})
}

func TestNewCmdAPI_TypedAndRawFields(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var got map[string]any
	captureBody(t, reg, "POST", "/api/v1/repos/o/r/issues", &got)

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "repos/o/r/issues",
		"-f", "raw=1", "--raw-field", "title=a, b",
		"-F", "n=1", "--field", "ok=true", "-F", "no=false", "-F", "none=null", "-F", "s=hello, world")
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"raw":   "1",
		"title": "a, b",
		"n":     float64(1),
		"ok":    true,
		"no":    false,
		"none":  nil,
		"s":     "hello, world",
	}, got)
}

func TestNewCmdAPI_FieldFromFile(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var got map[string]any
	captureBody(t, reg, "POST", "/api/v1/repos/o/r/issues", &got)

	file := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(file, []byte("line 1\nline 2, with comma\n"), 0o600))

	ios, stdin, _, _ := iostreams.Test()
	stdin.WriteString("from stdin")
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "repos/o/r/issues", "-F", "body=@"+file, "-F", "title=@-", "-f", "raw=@"+file)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"body":  "line 1\nline 2, with comma\n",
		"title": "from stdin",
		"raw":   "@" + file,
	}, got)
}

func TestNewCmdAPI_FieldFileMissing(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "repos/o/r/issues", "-F", "skipped", "-F", "body=@"+filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `error parsing "body" value`)
	assert.Empty(t, reg.Requests)
}

func TestNewCmdAPI_Input(t *testing.T) {
	file := filepath.Join(t.TempDir(), "body.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"title":"from file"}`), 0o600))

	tests := []struct {
		name      string
		args      []string
		stdin     string
		wantPath  string
		wantQuery string
		wantType  string
		wantBody  string
	}{
		{
			name:     "file",
			args:     []string{"--input", file},
			wantPath: "/api/v1/repos/o/r/issues",
			wantType: "application/json",
			wantBody: `{"title":"from file"}`,
		},
		{
			name:     "stdin",
			args:     []string{"--input", "-"},
			stdin:    `{"title":"from stdin"}`,
			wantPath: "/api/v1/repos/o/r/issues",
			wantType: "application/json",
			wantBody: `{"title":"from stdin"}`,
		},
		{
			name:      "fields become query parameters",
			args:      []string{"--input", file, "-f", "note=a, b", "-F", "n=1", "-F", "ok=true", "-F", "none=null"},
			wantPath:  "/api/v1/repos/o/r/issues",
			wantQuery: "n=1&none=&note=a%2C+b&ok=true",
			wantType:  "application/json",
			wantBody:  `{"title":"from file"}`,
		},
		{
			name:     "header overrides content type",
			args:     []string{"--input", file, "-H", "Content-Type: text/plain"},
			wantPath: "/api/v1/repos/o/r/issues",
			wantType: "text/plain",
			wantBody: `{"title":"from file"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			var gotBody string
			var gotReq *http.Request
			reg.Register(httpmock.REST("POST", tt.wantPath),
				func(req *http.Request) (*http.Response, error) {
					gotReq = req
					b, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					gotBody = string(b)
					return httpmock.StringResponse(http.StatusCreated, `{}`)(req)
				})

			ios, stdin, _, _ := iostreams.Test()
			stdin.WriteString(tt.stdin)
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			err := runAPICmd(t, f, append([]string{"repos/o/r/issues"}, tt.args...)...)
			require.NoError(t, err)

			require.NotNil(t, gotReq)
			assert.Equal(t, tt.wantBody, gotBody)
			assert.Equal(t, tt.wantQuery, gotReq.URL.RawQuery)
			assert.Equal(t, tt.wantType, gotReq.Header.Get("Content-Type"))
		})
	}
}

func TestNewCmdAPI_InputKeepsExistingQuery(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("PATCH", "/api/v1/repos/o/r/issues/1"),
		httpmock.StringResponse(http.StatusOK, `{}`))

	ios, stdin, _, _ := iostreams.Test()
	stdin.WriteString(`{}`)
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "repos/o/r/issues/1?a=1", "-X", "PATCH", "--input", "-", "-f", "b=2")
	require.NoError(t, err)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "a=1&b=2", reg.Requests[0].URL.RawQuery)
}

func TestNewCmdAPI_InputMissingFile(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
	err := runAPICmd(t, f, "repos/o/r/issues", "--input", filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading --input")
	assert.Empty(t, reg.Requests)
}
