package read

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

func TestReadRun_MarkAll(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("PUT", "/api/v1/notifications"),
		httpmock.StringResponse(http.StatusResetContent, `[]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ReadOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		All:        true,
	}

	err := ReadRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "All notifications marked as read")
}

func TestReadRun_MarkSingle(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("PATCH", "/api/v1/notifications/threads/42"),
		httpmock.StringResponse(http.StatusResetContent, ``),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ReadOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		ThreadID:   42,
	}

	err := ReadRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Notification #42 marked as read")
}

func TestReadRun_NoContent(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("PATCH", "/api/v1/notifications/threads/9"),
		httpmock.StringResponse(http.StatusNoContent, ``),
	)

	ios, _, stdout, _ := iostreams.Test()
	err := ReadRun(&ReadOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		ThreadID:   9,
	})
	require.NoError(t, err)
	assert.Equal(t, "Notification #9 marked as read\n", stdout.String())
}

func TestReadRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		respond httpmock.Responder
		wantErr string
	}{
		{
			name:    "not found",
			respond: httpmock.StringResponse(http.StatusNotFound, `{"message":"not found"}`),
			wantErr: "failed to mark as read (HTTP 404)",
		},
		{
			name: "transport error",
			respond: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("PATCH", "/api/v1/notifications/threads/42"), tt.respond)

			ios, _, stdout, _ := iostreams.Test()
			err := ReadRun(&ReadOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				ThreadID:   42,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestReadRun_InvalidHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := ReadRun(&ReadOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       "bad host\x7f",
		Token:      "test-token",
		All:        true,
	})
	require.Error(t, err)
}

func TestNewCmdRead_RunE(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		method  string
		path    string
		wantOut string
	}{
		{
			name:    "single thread",
			args:    []string{"42"},
			method:  "PATCH",
			path:    "/api/v1/notifications/threads/42",
			wantOut: "Notification #42 marked as read\n",
		},
		{
			name:    "all",
			args:    []string{"--all"},
			method:  "PUT",
			path:    "/api/v1/notifications",
			wantOut: "All notifications marked as read\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(httpmock.REST(tt.method, tt.path), httpmock.StringResponse(http.StatusResetContent, ``))

			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			cmd := NewCmdRead(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			require.Len(t, reg.Requests, 1)
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
		})
	}
}

func TestNewCmdRead_ArgErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "non-numeric id", args: []string{"abc"}, wantErr: "invalid thread ID: abc"},
		{name: "no id and no --all", args: []string{}, wantErr: "specify a thread ID or --all"},
		{name: "zero id", args: []string{"0"}, wantErr: "specify a thread ID or --all"},
		{name: "too many args", args: []string{"1", "2"}, wantErr: "accepts at most 1 arg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)

			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			cmd := NewCmdRead(f)
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

func TestNewCmdRead_AuthError(t *testing.T) {
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	cmd := NewCmdRead(f)
	cmd.SetArgs([]string{"--all"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config")
}
