package fork

import (
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/upstream-org/project/forks"),
		httpmock.StringResponse(http.StatusAccepted, `{"full_name":"john/project","html_url":"https://app.copia.io/john/project","clone_url":"https://app.copia.io/john/project.git"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ForkOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "upstream-org",
		Repo:       "project",
	}

	err := ForkRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "john/project")
}

func TestForkRun_ToOrg(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/upstream-org/project/forks"),
		httpmock.StringResponse(http.StatusAccepted, `{"full_name":"my-org/project","html_url":"https://app.copia.io/my-org/project"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &ForkOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "upstream-org",
		Repo:       "project",
		Org:        "my-org",
	}

	err := ForkRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "my-org/project")
}

func TestForkRun_NotFound(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantErr     string
	}{
		{
			name:        "no fork route",
			contentType: "text/plain; charset=utf-8",
			body:        "404 page not found\n",
			wantErr:     "Copia does not offer forking through its API",
		},
		{
			name:        "repository not found",
			contentType: "application/json;charset=utf-8",
			body:        `{"message":"The target couldn't be found.","errors":[]}`,
			wantErr:     `failed to fork repository (HTTP 404): {"message":"The target couldn't be found.","errors":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			reg.Register(
				httpmock.REST("POST", "/api/v1/repos/upstream-org/project/forks"),
				httpmock.WithHeader(httpmock.StringResponse(http.StatusNotFound, tt.body), "Content-Type", tt.contentType),
			)

			ios, _, stdout, _ := iostreams.Test()

			opts := &ForkOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "upstream-org",
				Repo:       "project",
			}

			err := ForkRun(opts)
			require.EqualError(t, err, tt.wantErr)
			assert.Empty(t, stdout.String())
		})
	}
}
