package lock

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const issuePath = "/api/v1/repos/my-org/my-repo/issues/12"

func runOpts(reg *httpmock.Registry, reason string) (*LockOptions, *bytes.Buffer, *bytes.Buffer) {
	ios, _, stdout, stderr := iostreams.Test()
	return &LockOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Reason:     reason,
	}, stdout, stderr
}

// recordBody answers with status and keeps the request body in *body.
func recordBody(body *string, status int) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		*body = string(b)
		return httpmock.StringResponse(status, "")(req)
	}
}

func TestLockRun(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		issue      string
		lockStatus int
		lockBody   string
		wantBody   string
		wantOut    string
		wantErr    string
	}{
		{
			name:       "without a reason",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusNoContent,
			wantBody:   `{}`,
			wantOut:    "Locked: Issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:       "off_topic",
			reason:     "off_topic",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusNoContent,
			wantBody:   `{"lock_reason":"Off-topic"}`,
			wantOut:    "Locked as OFF_TOPIC: Issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:       "resolved",
			reason:     "resolved",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusNoContent,
			wantBody:   `{"lock_reason":"Resolved"}`,
			wantOut:    "Locked as RESOLVED: Issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:       "spam",
			reason:     "spam",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusNoContent,
			wantBody:   `{"lock_reason":"Spam"}`,
			wantOut:    "Locked as SPAM: Issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:       "too_heated",
			reason:     "too_heated",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusNoContent,
			wantBody:   `{"lock_reason":"Too heated"}`,
			wantOut:    "Locked as TOO_HEATED: Issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:    "already locked sends nothing",
			issue:   `{"title":"Broken","is_locked":true}`,
			wantErr: "issue my-org/my-repo#12 (Broken) is already locked; unlock it first to lock it with another reason",
		},
		{
			name:       "forbidden",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusForbidden,
			wantBody:   `{}`,
			wantErr:    "cannot lock issue my-org/my-repo#12: permission denied (HTTP 403); locking an issue needs write access to the repository's issues",
		},
		{
			name:       "lock endpoint not served",
			issue:      `{"title":"Broken","is_locked":false}`,
			lockStatus: http.StatusNotFound,
			wantBody:   `{}`,
			wantErr:    "app.copia.io does not support locking issues (HTTP 404 on PUT /repos/my-org/my-repo/issues/12/lock)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, tt.issue))
			var body string
			if tt.lockStatus != 0 {
				reg.Register(httpmock.REST("PUT", issuePath+"/lock"), recordBody(&body, tt.lockStatus))
			}

			opts, stdout, stderr := runOpts(reg, tt.reason)
			err := LockRun(opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Empty(t, stderr.String())
			if tt.lockStatus != 0 {
				assert.JSONEq(t, tt.wantBody, body)
				assert.Equal(t, "token test-token", reg.Requests[1].Header.Get("Authorization"))
			} else {
				assert.Len(t, reg.Requests, 1, "no lock request")
			}
		})
	}
}

func TestLockRun_InvalidReason(t *testing.T) {
	reg := &httpmock.Registry{}
	opts, stdout, _ := runOpts(reg, "boring")
	require.EqualError(t, LockRun(opts), `invalid reason "boring"; use one of off_topic, resolved, spam, too_heated`)
	assert.Empty(t, stdout.String())
	assert.Empty(t, reg.Requests, "no request for an invalid reason")
}

func TestLockRun_Errors(t *testing.T) {
	tests := []struct {
		name    string
		issue   httpmock.Responder
		lock    httpmock.Responder
		wantErr string
	}{
		{
			name:    "issue not found",
			issue:   httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "issue my-org/my-repo#12 not found",
		},
		{
			name:    "issue read fails",
			issue:   httpmock.StringResponse(http.StatusInternalServerError, `{}`),
			wantErr: "failed to read issue my-org/my-repo#12 (HTTP 500)",
		},
		{
			name:    "invalid issue JSON",
			issue:   httpmock.StringResponse(http.StatusOK, `{`),
			wantErr: "unexpected end of JSON input",
		},
		{
			name:    "transport error",
			issue:   func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
			wantErr: "boom",
		},
		{
			name:    "server message",
			issue:   httpmock.StringResponse(http.StatusOK, `{"title":"Broken","is_locked":false}`),
			lock:    httpmock.StringResponse(http.StatusInternalServerError, `{"message":"database is locked"}`),
			wantErr: "failed to lock issue my-org/my-repo#12 (HTTP 500): database is locked",
		},
		{
			name:    "status without message",
			issue:   httpmock.StringResponse(http.StatusOK, `{"title":"Broken","is_locked":false}`),
			lock:    httpmock.StringResponse(http.StatusBadGateway, `bad gateway`),
			wantErr: "failed to lock issue my-org/my-repo#12 (HTTP 502)",
		},
		{
			name:    "lock transport error",
			issue:   httpmock.StringResponse(http.StatusOK, `{"title":"Broken","is_locked":false}`),
			lock:    func(*http.Request) (*http.Response, error) { return nil, errors.New("reset") },
			wantErr: "reset",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), tt.issue)
			if tt.lock != nil {
				reg.Register(httpmock.REST("PUT", issuePath+"/lock"), tt.lock)
			}

			opts, stdout, _ := runOpts(reg, "")
			err := LockRun(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestUnlockRun(t *testing.T) {
	tests := []struct {
		name       string
		issue      string
		unlock     httpmock.Responder
		wantOut    string
		wantErrOut string
		wantErr    string
	}{
		{
			name:    "unlocks a locked issue",
			issue:   `{"title":"Broken","is_locked":true}`,
			unlock:  httpmock.StringResponse(http.StatusNoContent, ""),
			wantOut: "Unlocked: Issue my-org/my-repo#12 (Broken)\n",
		},
		{
			name:       "not locked sends nothing",
			issue:      `{"title":"Broken","is_locked":false}`,
			wantErrOut: "! Issue my-org/my-repo#12 (Broken) is not locked\n",
		},
		{
			name:    "forbidden",
			issue:   `{"title":"Broken","is_locked":true}`,
			unlock:  httpmock.StringResponse(http.StatusForbidden, `{}`),
			wantErr: "cannot unlock issue my-org/my-repo#12: permission denied (HTTP 403); unlocking an issue needs write access to the repository's issues",
		},
		{
			name:    "unlock endpoint not served",
			issue:   `{"title":"Broken","is_locked":true}`,
			unlock:  httpmock.StringResponse(http.StatusNotFound, `{}`),
			wantErr: "app.copia.io does not support unlocking issues (HTTP 404 on DELETE /repos/my-org/my-repo/issues/12/lock)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, tt.issue))
			if tt.unlock != nil {
				reg.Register(httpmock.REST("DELETE", issuePath+"/lock"), tt.unlock)
			}

			opts, stdout, stderr := runOpts(reg, "")
			err := UnlockRun(opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}

func TestUnlockRun_IssueNotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusNotFound, `{}`))

	opts, stdout, _ := runOpts(reg, "")
	require.EqualError(t, UnlockRun(opts), "issue my-org/my-repo#12 not found")
	assert.Empty(t, stdout.String())
	assert.Len(t, reg.Requests, 1, "no unlock request for a missing issue")
}

func runCmd(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readyFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
}

func TestNewCmdLock_ReasonFlag(t *testing.T) {
	for _, flag := range []string{"--reason", "-r"} {
		t.Run(flag, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			var body string
			reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{"title":"Broken","is_locked":false}`))
			reg.Register(httpmock.REST("PUT", issuePath+"/lock"), recordBody(&body, http.StatusNoContent))

			ios, _, stdout, _ := iostreams.Test()
			require.NoError(t, runCmd(t, NewCmdLock(readyFactory(ios)), "#12", flag, "spam"))
			assert.Equal(t, "Locked as SPAM: Issue my-org/my-repo#12 (Broken)\n", stdout.String())
			assert.JSONEq(t, `{"lock_reason":"Spam"}`, body)
		})
	}
}

func TestNewCmdLock_InvalidReasonSendsNothing(t *testing.T) {
	reg := &httpmock.Registry{}
	httpmock.ReplaceDefaultTransport(t, reg)
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
	}
	err := runCmd(t, NewCmdLock(f), "12", "--reason", "boring")
	require.EqualError(t, err, `invalid reason "boring"; use one of off_topic, resolved, spam, too_heated`)
	assert.Empty(t, reg.Requests)
}

func TestNewCmdLock_URLArgument(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/other/thing/issues/5"),
		httpmock.StringResponse(http.StatusOK, `{"title":"Elsewhere","is_locked":false}`))
	reg.Register(httpmock.REST("PUT", "/api/v1/repos/other/thing/issues/5/lock"),
		httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, NewCmdLock(readyFactory(ios)), "https://app.copia.io/other/thing/issues/5"))
	assert.Equal(t, "Locked: Issue other/thing#5 (Elsewhere)\n", stdout.String())
}

func TestNewCmdUnlock_Number(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", issuePath), httpmock.StringResponse(http.StatusOK, `{"title":"Broken","is_locked":true}`))
	reg.Register(httpmock.REST("DELETE", issuePath+"/lock"), httpmock.StringResponse(http.StatusNoContent, ""))

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, runCmd(t, NewCmdUnlock(readyFactory(ios)), "12"))
	assert.Equal(t, "Unlocked: Issue my-org/my-repo#12 (Broken)\n", stdout.String())
}

func TestNewCmdUnlock_NoReasonFlag(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := runCmd(t, NewCmdUnlock(readyFactory(ios)), "12", "--reason", "spam")
	require.EqualError(t, err, "unknown flag: --reason")
}

func TestNewCmd_ArgumentErrors(t *testing.T) {
	for name, newCmd := range map[string]func(*cmdutil.Factory) *cobra.Command{"lock": NewCmdLock, "unlock": NewCmdUnlock} {
		t.Run(name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			require.EqualError(t, runCmd(t, newCmd(readyFactory(ios)), "abc"), "invalid issue number or URL: abc")
			require.Error(t, runCmd(t, newCmd(readyFactory(ios))))
			require.EqualError(t, runCmd(t, newCmd(readyFactory(ios)), "https://copia.example.com/other/thing/issues/5"),
				"issue URL host copia.example.com does not match the active host app.copia.io")

			t.Setenv("COPIA_TOKEN", "")
			t.Setenv("COPIA_HOST", "")
			noAuth := &cmdutil.Factory{
				IOStreams: ios,
				Repo:      "my-org/my-repo",
				Config:    func() (*config.Config, error) { return nil, errors.New("no config") },
			}
			require.EqualError(t, runCmd(t, newCmd(noAuth), "12"), "no config")
		})
	}
}

func TestNewCmd_RepoFailure(t *testing.T) {
	for name, newCmd := range map[string]func(*cmdutil.Factory) *cobra.Command{"lock": NewCmdLock, "unlock": NewCmdUnlock} {
		t.Run(name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}
			err := runCmd(t, newCmd(f), "12")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "could not determine repository")
		})
	}
}
