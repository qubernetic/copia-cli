package view

import (
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewRun_StartsPager(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org"), httpmock.StringResponse(http.StatusOK, `{"username":"my-org","full_name":"My Organization"}`))

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetPager("copia-no-such-pager")

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Name:       "my-org",
	}

	require.NoError(t, ViewRun(opts))
	assert.Contains(t, stderr.String(), "failed to start pager: ")
	assert.Contains(t, stdout.String(), "My Organization")
}
