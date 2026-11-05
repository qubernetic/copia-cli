package diff

import (
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffRun_StartsPager(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/pulls/7.diff"), httpmock.StringResponse(http.StatusOK, `+import "fmt"`))

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetPager("copia-no-such-pager")

	opts := &DiffOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
	}

	require.NoError(t, DiffRun(opts))
	assert.Contains(t, stderr.String(), "failed to start pager: ")
	assert.Contains(t, stdout.String(), "+import \"fmt\"")
}
