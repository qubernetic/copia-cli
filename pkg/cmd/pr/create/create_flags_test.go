package create

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCreateSent runs pr create with args against a mocked create endpoint
// and returns the decoded request body.
func runCreateSent(t *testing.T, ios *iostreams.IOStreams, args ...string) map[string]string {
	t.Helper()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls"),
		httpmock.StringResponse(http.StatusCreated,
			`{"number":8,"title":"x","html_url":"https://app.copia.io/my-org/my-repo/pulls/8"}`))

	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}
	require.NoError(t, runCreate(t, f, args...))

	require.Len(t, reg.Requests, 1)
	var sent map[string]string
	require.NoError(t, json.NewDecoder(reg.Requests[0].Body).Decode(&sent))
	return sent
}

func TestCreate_Draft(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{title: "feat: x", want: "WIP: feat: x"},
		{title: "WIP: feat: x", want: "WIP: feat: x"},
		{title: "wip:feat: x", want: "wip:feat: x"},
		{title: "[WIP] feat: x", want: "[WIP] feat: x"},
		{title: "[wip] feat: x", want: "[wip] feat: x"},
		{title: "WIPE: x", want: "WIP: WIPE: x"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			sent := runCreateSent(t, ios, "--draft", "--title", tt.title, "--head", "feature/x")
			assert.Equal(t, tt.want, sent["title"])
		})
	}

	t.Run("short flag", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		sent := runCreateSent(t, ios, "-d", "-t", "feat: x", "--head", "feature/x")
		assert.Equal(t, "WIP: feat: x", sent["title"])
	})
}
