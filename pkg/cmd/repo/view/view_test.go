package view

import (
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"),
		httpmock.StringResponse(http.StatusOK, `{
			"full_name":"my-org/my-repo",
			"description":"Main PLC project",
			"html_url":"https://app.copia.io/my-org/my-repo",
			"private":false,
			"default_branch":"main",
			"stars_count":5,
			"forks_count":2,
			"open_issues_count":3
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "my-org/my-repo")
	assert.Contains(t, stdout.String(), "Main PLC project")
}

func TestViewRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo"),
		httpmock.StringResponse(http.StatusOK, `{
			"full_name":"my-org/my-repo","description":"PLC","html_url":"https://app.copia.io/my-org/my-repo",
			"private":false,"default_branch":"main","stars_count":0,"forks_count":0,"open_issues_count":0
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		JSON:       cmdutil.JSONFlags{Fields: []string{"full_name", "description"}},
	}

	err := ViewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"full_name"`)
}

func TestViewRun_NotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/missing"),
		httpmock.StringResponse(http.StatusNotFound, `{}`),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "missing",
	}

	err := ViewRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
