package unarchive

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoPath = "/api/v1/repos/my-org/my-repo"

func runCmd(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdUnarchive(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdUnarchive(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	var payload map[string]any
	reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo","archived":true}`))
	reg.Register(httpmock.REST("PATCH", repoPath), func(req *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &payload))
		return httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo"}`)(req)
	})

	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}

	require.NoError(t, runCmd(f, "--yes"))
	assert.Equal(t, map[string]any{"archived": false}, payload)
	assert.Equal(t, "Unarchived repository my-org/my-repo\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNewCmdUnarchive_NotArchived(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)
	reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(http.StatusOK, `{"full_name":"my-org/my-repo","archived":false}`))

	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	require.NoError(t, runCmd(f, "my-org/my-repo", "-y"))
	assert.Empty(t, stdout.String())
	assert.Equal(t, "! Repository my-org/my-repo is not archived\n", stderr.String())
}

func TestNewCmdUnarchive_NoYes(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	httpmock.ReplaceDefaultTransport(t, reg)

	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token"}

	require.EqualError(t, runCmd(f, "my-org/my-repo"), "--yes required when not running interactively")
	assert.Empty(t, stdout.String())
}
