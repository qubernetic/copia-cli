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
	{"id":7,"primary_key_id":"","key_id":"3AA5C34371567BD2","public_key":"xsBNBGNpcGFjbGl0ZXN0a2V5b25lAAAA","emails":[{"email":"dev@example.com","verified":true},{"email":"work@example.com","verified":true}],"subkeys":[],"can_sign":true,"verified":true,"created_at":"2026-11-01T10:00:00Z","expires_at":"0001-01-01T00:00:00Z"},
	{"id":9,"primary_key_id":"","key_id":"4BB6D45482678CE3","public_key":"xsBNBGNpcGFjbGl0ZXN0a2V5dHdvAAAA","emails":[],"subkeys":[],"can_sign":false,"verified":false,"created_at":"2026-11-07T10:00:00Z","expires_at":"2027-11-07T10:00:00Z"}
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
			wantOut: "dev@example.com, work@example.com\t3AA5C34371567BD2\txsBNBGNpcGFjbGl0ZXN0a2V5b25lAAAA\t2026-11-01T10:00:00Z\t0001-01-01T00:00:00Z\n" +
				"\t4BB6D45482678CE3\txsBNBGNpcGFjbGl0ZXN0a2V5dHdvAAAA\t2026-11-07T10:00:00Z\t2027-11-07T10:00:00Z\n",
		},
		{
			name:  "on a terminal with a header, the time in words and Never",
			tty:   true,
			width: 200,
			body:  twoKeys,
			wantOut: "EMAIL                              KEY ID            PUBLIC KEY                        ADDED       EXPIRES\n" +
				"dev@example.com, work@example.com  3AA5C34371567BD2  xsBNBGNpcGFjbGl0ZXN0a2V5b25lAAAA  7 days ago  Never\n" +
				"                                   4BB6D45482678CE3  xsBNBGNpcGFjbGl0ZXN0a2V5dHdvAAAA  1 day ago   2027-11-07\n",
		},
		{
			name:  "on a narrow terminal the public key is cut in the middle",
			tty:   true,
			width: 72,
			body:  `[{"id":7,"key_id":"3AA5C34371567BD2","public_key":"xsBNBGNpcGFjbGl0ZXN0a2V5b25lAAAA","emails":[{"email":"a@b.c"}],"created_at":"2026-11-08T09:59:00Z","expires_at":"1970-01-01T00:00:00Z"}]`,
			wantOut: "EMAIL  KEY ID            PUBLIC KEY                ADDED         EXPIRES\n" +
				"a@b.c  3AA5C34371567BD2  xsBNBGNpcGF...V5b25lAAAA  1 minute ago  Never\n",
		},
		{
			name:       "no keys",
			body:       `[]`,
			wantErrOut: "no GPG keys present in the Copia account\n",
		},
		{
			name:    "no keys with --json prints an empty list",
			body:    `[]`,
			json:    []string{"id"},
			wantOut: "[]\n",
		},
		{
			name: "--json with the API's field names",
			body: twoKeys,
			json: validJSONFields,
			wantOut: `[
				{"id":7,"key_id":"3AA5C34371567BD2","primary_key_id":"","emails":[{"email":"dev@example.com","verified":true},{"email":"work@example.com","verified":true}],"public_key":"xsBNBGNpcGFjbGl0ZXN0a2V5b25lAAAA","created_at":"2026-11-01T10:00:00Z","expires_at":"0001-01-01T00:00:00Z","verified":true,"can_sign":true},
				{"id":9,"key_id":"4BB6D45482678CE3","primary_key_id":"","emails":[],"public_key":"xsBNBGNpcGFjbGl0ZXN0a2V5dHdvAAAA","created_at":"2026-11-07T10:00:00Z","expires_at":"2027-11-07T10:00:00Z","verified":false,"can_sign":false}
			]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.QueryMatcher("GET", "/api/v1/user/gpg_keys", url.Values{"page": {"1"}, "limit": {"50"}}),
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
		first = append(first, fmt.Sprintf(`{"id":%d,"key_id":"K%d","public_key":"P%d","emails":[],"created_at":"2026-11-01T10:00:00Z","expires_at":"0001-01-01T00:00:00Z"}`, i, i, i))
	}
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/user/gpg_keys", url.Values{"page": {"1"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, "["+strings.Join(first, ",")+"]"),
	)
	reg.Register(
		httpmock.QueryMatcher("GET", "/api/v1/user/gpg_keys", url.Values{"page": {"2"}, "limit": {"50"}}),
		httpmock.StringResponse(http.StatusOK, `[{"id":51,"key_id":"LAST","public_key":"P51","emails":[],"created_at":"2026-11-01T10:00:00Z","expires_at":"0001-01-01T00:00:00Z"}]`),
	)

	ios, _, stdout, _ := iostreams.Test()
	require.NoError(t, ListRun(newOpts(reg, ios)))
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Len(t, lines, 51)
	assert.True(t, strings.HasPrefix(lines[50], "\tLAST\t"))
}

func TestListRun_HTTPError(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "with Gitea's message",
			body:    `{"message":"token does not have at least one of required scope(s): [read:user]"}`,
			wantErr: "failed to list GPG keys (HTTP 403): token does not have at least one of required scope(s): [read:user]",
		},
		{
			name:    "without a message",
			body:    ``,
			wantErr: "failed to list GPG keys (HTTP 403)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(httpmock.REST("GET", "/api/v1/user/gpg_keys"), httpmock.StringResponse(http.StatusForbidden, tt.body))

			ios, _, stdout, _ := iostreams.Test()
			err := ListRun(newOpts(reg, ios))
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestTruncateMiddle(t *testing.T) {
	assert.Equal(t, "abcdef", truncateMiddle(6, "abcdef"))
	assert.Equal(t, "ab...ef", truncateMiddle(7, "abcdefghef"))
	assert.Equal(t, "abc", truncateMiddle(3, "abcdefgh"))
	assert.Equal(t, "ab...h", truncateMiddle(6, "abcdefgh"))
}

func TestExpires(t *testing.T) {
	assert.Equal(t, "Never", expires("0001-01-01T00:00:00Z"))
	assert.Equal(t, "Never", expires("1970-01-01T00:00:00Z"))
	assert.Equal(t, "Never", expires("1970-01-01T01:00:00+01:00"))
	assert.Equal(t, "2027-11-07", expires("2027-11-07T10:00:00Z"))
	assert.Equal(t, "soon", expires("soon"))
}

func TestTimeAgo_KeepsAnUnparsableTime(t *testing.T) {
	assert.Equal(t, "yesterday", timeAgo(time.Now(), "yesterday"))
}

func TestNewCmdList(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdList(&cmdutil.Factory{IOStreams: ios})
	assert.Equal(t, []string{"ls"}, cmd.Aliases)
	assert.NotNil(t, cmd.Flags().Lookup("json"))

	cmd.SetArgs([]string{"extra"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	assert.Error(t, cmd.Execute())
}

func TestNewCmdList_Runs(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", "/api/v1/user/gpg_keys"), httpmock.StringResponse(http.StatusOK, twoKeys))

	ios, _, stdout, _ := iostreams.Test()
	cmd := NewCmdList(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "flag-token"})
	cmd.SetArgs([]string{"--json", "id,key_id"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	require.NoError(t, cmd.Execute())
	assert.JSONEq(t, `[{"id":7,"key_id":"3AA5C34371567BD2"},{"id":9,"key_id":"4BB6D45482678CE3"}]`, stdout.String())
	assert.Equal(t, "token flag-token", reg.Requests[0].Header.Get("Authorization"))
}
