package shared

import (
	"errors"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	repoLabelsPath = "/api/v1/repos/my-org/my-repo/labels"
	orgLabelsPath  = "/api/v1/orgs/my-org/labels"
)

func resolve(reg *httpmock.Registry, names ...string) ([]int64, error) {
	client := &http.Client{Transport: reg}
	return ResolveLabelIDs(client, "app.copia.io", "test-token", "my-org", "my-repo", names)
}

func TestResolveLabelIDs_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"},{"id":2,"name":"dup"}]`))
	reg.Register(httpmock.REST("GET", orgLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":90,"name":"org"},{"id":91,"name":"dup"}]`))

	ids, err := resolve(reg, "org", "bug", "dup")
	require.NoError(t, err)
	assert.Equal(t, []int64{90, 1, 2}, ids)
}

func TestResolveLabelIDs_RepoLabelsError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusInternalServerError, `{}`))

	_, err := resolve(reg, "bug")
	require.EqualError(t, err, "API error (HTTP 500)")
	assert.Len(t, reg.Requests, 1)
}

func TestResolveLabelIDs_RepoLabelsMalformedJSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusOK, `{not json`))

	_, err := resolve(reg, "bug")
	require.Error(t, err)
}

func TestResolveLabelIDs_OrgLabelsTransportError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", orgLabelsPath), func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})

	_, err := resolve(reg, "bug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestResolveLabelIDs_OrgNotFoundUsesRepoLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", repoLabelsPath), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", orgLabelsPath), httpmock.StringResponse(http.StatusNotFound, `{}`))

	ids, err := resolve(reg, "bug")
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, ids)

	_, err = resolve(reg, "missing")
	require.EqualError(t, err, `label "missing" not found in repository or organization`)
}
