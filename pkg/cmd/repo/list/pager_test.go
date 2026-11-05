package list

import (
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRun_StartsPager(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user/repos"), httpmock.StringResponse(http.StatusOK, `[{"full_name":"john/plc-project","description":"PLC code"}]`))

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetPager("copia-no-such-pager")

	opts := &ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Limit:      30,
	}

	require.NoError(t, ListRun(opts))
	assert.Contains(t, stderr.String(), "failed to start pager: ")
	assert.Contains(t, stdout.String(), "john/plc-project")
}
