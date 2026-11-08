package list

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoKeys = `[
	{"id":7,"title":"laptop","key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey1","fingerprint":"SHA256:one","created_at":"2026-11-01T10:00:00Z","last_used_at":"2026-11-02T10:00:00Z"},
	{"id":9,"title":"desktop","key":"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQKey2","fingerprint":"SHA256:two","created_at":"2026-11-07T10:00:00Z","last_used_at":"0001-01-01T00:00:00Z"}
]`

func newOpts(reg *httpmock.Registry, ios *iostreams.IOStreams) *ListOptions {
	return &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Now:        func() time.Time { return time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC) },
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		width      int
		body       string
		json       []string
		wantOut    string
		wantErrOut string
	}{
		{
			name: "without a terminal in gh's column order",
			body: twoKeys,
			wantOut: "laptop\tssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey1\t2026-11-01T10:00:00Z\t7\tauthentication\n" +
				"desktop\tssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQKey2\t2026-11-07T10:00:00Z\t9\tauthentication\n",
		},
		{
			name:  "on a terminal with a header and the time in words",
			tty:   true,
			width: 200,
			body:  twoKeys,
			wantOut: "TITLE    ID  KEY                                         TYPE            ADDED\n" +
				"laptop   7   ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey1   authentication  7 days ago\n" +
				"desktop  9   ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQKey2  authentication  1 day ago\n",
		},
		{
			name:  "on a narrow terminal the key is cut in the middle",
			tty:   true,
			width: 70,
			body:  `[{"id":7,"title":"laptop","key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey1","created_at":"2026-11-08T09:59:00Z"}]`,
			wantOut: "TITLE   ID  KEY                           TYPE            ADDED\n" +
				"laptop  7   ssh-ed25519 A...TE5AAAAIKey1  authentication  1 minute ago\n",
		},
		{
			name:       "no keys",
			body:       `[]`,
			wantErrOut: "no SSH keys present in the Copia account\n",
		},
		{
			name:    "no keys with --json prints an empty list",
			body:    `[]`,
			json:    []string{"id"},
			wantOut: "[]\n",
		},
		{
			name:    "--json with the API's field names",
			body:    twoKeys,
			json:    []string{"id", "title", "fingerprint", "last_used_at"},
			wantOut: `[{"fingerprint":"SHA256:one","id":7,"last_used_at":"2026-11-02T10:00:00Z","title":"laptop"},{"fingerprint":"SHA256:two","id":9,"last_used_at":"0001-01-01T00:00:00Z","title":"desktop"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.QueryMatcher("GET", "/api/v1/user/keys", url.Values{"page": {"1"}, "limit": {"50"}}),
				httpmock.StringResponse(http.StatusOK, tt.body),
			)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			if tt.width > 0 {
				ios.SetTerminalWidth(tt.width)
			}
			opts := newOpts(reg, ios)
			opts.JSON = cmdutil.JSONFlags{Fields: tt.json}

			require.NoError(t, ListRun(opts))
			if tt.json != nil && tt.wantOut != "[]\n" {
				assert.JSONEq(t, tt.wantOut, stdout.String())
			} else {
				assert.Equal(t, tt.wantOut, stdout.String())
			}
			assert.Equal(t, tt.wantErrOut, stderr.String())
			assert.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
		})
	}
}

func TestListRun_ReadsEveryPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	var first []string
	for i := 1; i <= 50; i++ {
		first = append(first, fmt.Sprintf(`{"id":%d,"title":"k%d","key":"ssh-ed25519 K%d","created_at":"2026-11-01T10:00:00Z"}`, i, i, i))
	}
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/user/keys", url.Values{"page": {"1"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, "["+strings.Join(first, ",")+"]"),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/user/keys", url.Values{"page": {"2"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, `[{"id":51,"title":"last","key":"ssh-ed25519 K51","created_at":"2026-11-01T10:00:00Z"}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, ListRun(newOpts(reg, ios)))
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Len(t, lines, 51)
	assert.True(t, strings.HasPrefix(lines[50], "last\t"))
}

func TestListRun_HTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.REST("GET", "/api/v1/user/keys"),
		httpmock.StringResponse(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [read:user]"}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	err := ListRun(newOpts(reg, ios))
	require.EqualError(t, err, "failed to list SSH keys (HTTP 403): token does not have at least one of required scope(s): [read:user]")
	assert.Empty(t, stdout.String())
}

func TestTruncateMiddle(t *testing.T) {
	assert.Equal(t, "abcdef", truncateMiddle(6, "abcdef"))
	assert.Equal(t, "ab...ef", truncateMiddle(7, "abcdefghef"))
	assert.Equal(t, "abc", truncateMiddle(3, "abcdefgh"))
	assert.Equal(t, "ab...h", truncateMiddle(6, "abcdefgh"))
}

func TestNewCmdList(t *testing.T) {
	f := &cmdutil.Factory{IOStreams: func() *iostreams.IOStreams { ios, _, _, _ := iostreams.Test(); return ios }()}
	cmd := NewCmdList(f)
	assert.Equal(t, []string{"ls"}, cmd.Aliases)
	assert.NotNil(t, cmd.Flags().Lookup("json"))

	cmd.SetArgs([]string{"extra"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	assert.Error(t, cmd.Execute())
}
