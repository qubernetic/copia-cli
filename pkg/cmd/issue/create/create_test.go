package create

import (
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues"),
		httpmock.StringResponse(http.StatusCreated, `{"number":13,"title":"Fix sensor mapping","html_url":"https://app.copia.io/my-org/my-repo/issues/13"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &CreateOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Title:      "Fix sensor mapping",
		Body:       "The sensor I/O mapping is incorrect.",
	}

	err := CreateRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "#13")
}

func TestCreateRun_MissingTitle(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &CreateOptions{IO: ios, Title: ""}

	err := CreateRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title required")
}

const createdIssue = `{"number":13,"title":"Fix sensor mapping","html_url":"https://app.copia.io/my-org/my-repo/issues/13"}`

// recordBody wraps a responder and stores the request body in *dst.
func recordBody(dst *string, responder httpmock.Responder) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		*dst = string(body)
		return responder(req)
	}
}

func labelOpts(reg *httpmock.Registry, labels ...string) *CreateOptions {
	ios, _, _, _ := iostreams.Test()
	return &CreateOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Title:      "Fix sensor mapping",
		Labels:     labels,
	}
}

func TestCreateRun_Labels_MixesRepoAndOrgLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":90,"name":"priority/high"}]`))
	var posted string
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues"),
		recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdIssue)),
	)

	err := CreateRun(labelOpts(reg, "bug", "priority/high"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Fix sensor mapping","labels":[1,90]}`, posted)
}

func TestCreateRun_Labels_RepoLabelWinsNameClash(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":90,"name":"bug"}]`))
	var posted string
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues"),
		recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdIssue)),
	)

	err := CreateRun(labelOpts(reg, "bug"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Fix sensor mapping","labels":[1]}`, posted)
}

func TestCreateRun_Labels_UserOwnedRepoSkipsOrgLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusNotFound, `{"message":"GetOrgByName"}`))
	var posted string
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues"),
		recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdIssue)),
	)

	err := CreateRun(labelOpts(reg, "bug"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Fix sensor mapping","labels":[1]}`, posted)
}

func TestCreateRun_Labels_UnknownLabel(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":90,"name":"priority/high"}]`))

	err := CreateRun(labelOpts(reg, "bug", "x"))
	require.EqualError(t, err, `label "x" not found in repository or organization`)
	assert.Len(t, reg.Requests, 2, "no issue is created")
}
