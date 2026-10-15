package edit

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditRun_SetTitle(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo/issues/12"),
		httpmock.StringResponse(http.StatusOK, `{"number":12,"title":"Updated title"}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Title:      "Updated title",
	}

	err := EditRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Updated issue #12")
}

func TestEditRun_AddLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"},{"id":2,"name":"feature"}]`),
	)
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[]`))
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues/12/labels"),
		httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		AddLabels:  []string{"bug"},
	}

	err := EditRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Updated issue #12")
}

func labelsJSON(first, n int) string {
	labels := make([]string, n)
	for i := range labels {
		id := first + i
		labels[i] = fmt.Sprintf(`{"id":%d,"name":"label-%d"}`, id, id)
	}
	return "[" + strings.Join(labels, ",") + "]"
}

func pageQuery(page, limit int) url.Values {
	return url.Values{"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(limit)}}
}

func TestEditRun_AddLabels_ResolvesLabelOnLaterPage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	path := "/api/v1/repos/my-org/my-repo/labels"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, labelsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, `[{"id":77,"name":"needs-review"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[]`))

	var posted string
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues/12/labels"),
		func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			posted = string(body)
			return httpmock.StringResponse(http.StatusOK, `[{"id":77,"name":"needs-review"}]`)(req)
		},
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		AddLabels:  []string{"needs-review", "label-3"},
	}

	err := EditRun(opts)
	require.NoError(t, err)
	assert.JSONEq(t, `{"labels":[77,3]}`, posted)
	assert.Contains(t, stdout.String(), "Updated issue #12")
}

func TestEditRun_AddLabels_UnknownLabel(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	path := "/api/v1/repos/my-org/my-repo/labels"
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(1, 50)), httpmock.StringResponse(http.StatusOK, labelsJSON(1, 50)))
	reg.Register(httpmock.QueryMatcher("GET", path, pageQuery(2, 50)), httpmock.StringResponse(http.StatusOK, labelsJSON(51, 3)))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[]`))

	ios, _, _, _ := iostreams.Test()

	opts := &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		AddLabels:  []string{"missing"},
	}

	err := EditRun(opts)
	require.EqualError(t, err, `label "missing" not found in repository or organization`)
	assert.Len(t, reg.Requests, 3)
}

func TestEditRun_SetAssignees(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo/issues/12"),
		httpmock.StringResponse(http.StatusOK, `{"number":12}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Assignees:  []string{"john", "jane"},
	}

	err := EditRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Updated issue #12")
}

func TestEditRun_SetMilestone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("PATCH", "/api/v1/repos/my-org/my-repo/issues/12"),
		httpmock.StringResponse(http.StatusOK, `{"number":12}`),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		Milestone:  1,
	}

	err := EditRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Updated issue #12")
}

func TestEditRun_NothingToEdit(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	opts := &EditOptions{
		IO:     ios,
		Number: 12,
	}

	err := EditRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to edit")
}

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

func labelOpts(reg *httpmock.Registry, labels ...string) *EditOptions {
	ios, _, _, _ := iostreams.Test()
	return &EditOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     12,
		AddLabels:  labels,
	}
}

func TestEditRun_AddLabels_MixesRepoAndOrgLabels(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":1,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"), httpmock.StringResponse(http.StatusOK, `[{"id":90,"name":"priority/high"}]`))
	var posted string
	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/issues/12/labels"),
		recordBody(&posted, httpmock.StringResponse(http.StatusOK, `[]`)),
	)

	err := EditRun(labelOpts(reg, "bug", "priority/high"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"labels":[1,90]}`, posted)
}
