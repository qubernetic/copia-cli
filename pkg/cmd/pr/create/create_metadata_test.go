package create

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/httpmock"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pullsPath     = "/api/v1/repos/my-org/my-repo/pulls"
	reviewersPath = "/api/v1/repos/my-org/my-repo/pulls/8/requested_reviewers"
	createdPR     = `{"number":8,"title":"feat: x","html_url":"https://app.copia.io/my-org/my-repo/pulls/8"}`
	createdOutput = "Created PR #8: feat: x\nhttps://app.copia.io/my-org/my-repo/pulls/8\n"
)

// recordBody wraps a responder and stores the request body it receives.
func recordBody(dst *string, next httpmock.Responder) httpmock.Responder {
	return func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		*dst = string(b)
		return next(req)
	}
}

func metadataFactory(t *testing.T, reg *httpmock.Registry) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	httpmock.ReplaceDefaultTransport(t, reg)
	ios, _, stdout, _ := iostreams.Test()
	return &cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"}, stdout
}

func countRequests(reg *httpmock.Registry, method, path string) int {
	n := 0
	for _, r := range reg.Requests {
		if r.Method == method && r.URL.Path == path {
			n++
		}
	}
	return n
}

func TestNewCmdCreate_Assignee(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusOK, `{"login":"monalisa"}`))
	var posted string
	reg.Register(httpmock.REST("POST", pullsPath), recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdPR)))

	f, _ := metadataFactory(t, reg)
	err := runCreate(t, f, "--title", "feat: x", "--head", "feature/x", "-a", "@me", "--assignee", "jane")
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"feat: x","base":"main","head":"feature/x","assignees":["monalisa","jane"]}`, posted)
}

func TestNewCmdCreate_Label(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"),
		httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"bug"}]`))
	reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"),
		httpmock.StringResponse(http.StatusOK, `[{"id":9,"name":"plc"}]`))
	var posted string
	reg.Register(httpmock.REST("POST", pullsPath), recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdPR)))

	f, _ := metadataFactory(t, reg)
	err := runCreate(t, f, "--title", "feat: x", "--head", "feature/x", "-l", "bug", "--label", "plc")
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"feat: x","base":"main","head":"feature/x","labels":[3,9]}`, posted)
}

func TestNewCmdCreate_Milestone(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/v1.0"),
		httpmock.StringResponse(http.StatusOK, `{"id":5,"title":"v1.0"}`))
	var posted string
	reg.Register(httpmock.REST("POST", pullsPath), recordBody(&posted, httpmock.StringResponse(http.StatusCreated, createdPR)))

	f, _ := metadataFactory(t, reg)
	err := runCreate(t, f, "--title", "feat: x", "--head", "feature/x", "-m", "v1.0")
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"feat: x","base":"main","head":"feature/x","milestone":5}`, posted)
}

func TestNewCmdCreate_UnknownMetadataFailsBeforeCreate(t *testing.T) {
	tests := []struct {
		name    string
		stub    func(reg *httpmock.Registry)
		args    []string
		wantErr string
	}{
		{
			name: "unknown label",
			stub: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/labels"),
					httpmock.StringResponse(http.StatusOK, `[{"id":3,"name":"bug"}]`))
				reg.Register(httpmock.REST("GET", "/api/v1/orgs/my-org/labels"),
					httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			args:    []string{"--label", "nope"},
			wantErr: `label "nope" not found in repository or organization`,
		},
		{
			name: "unknown milestone",
			stub: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "/api/v1/repos/my-org/my-repo/milestones/nope"),
					httpmock.StringResponse(http.StatusNotFound, `{}`))
			},
			args:    []string{"--milestone", "nope"},
			wantErr: `milestone "nope" not found in my-org/my-repo`,
		},
		{
			name: "@me lookup fails",
			stub: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "/api/v1/user"), httpmock.StringResponse(http.StatusUnauthorized, `{}`))
			},
			args:    []string{"--assignee", "@me"},
			wantErr: "failed to resolve @me (HTTP 401)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stub(reg)

			f, _ := metadataFactory(t, reg)
			err := runCreate(t, f, append([]string{"--title", "feat: x", "--head", "feature/x"}, tt.args...)...)
			require.EqualError(t, err, tt.wantErr)
			assert.Zero(t, countRequests(reg, "POST", pullsPath))
		})
	}
}

func TestNewCmdCreate_Reviewer(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("POST", pullsPath), httpmock.StringResponse(http.StatusCreated, createdPR))
	var requested string
	reg.Register(httpmock.REST("POST", reviewersPath), recordBody(&requested, httpmock.StringResponse(http.StatusCreated, `[]`)))

	f, stdout := metadataFactory(t, reg)
	err := runCreate(t, f, "--title", "feat: x", "--head", "feature/x",
		"-r", "jane", "--reviewer", "my-org/plc-team", "--reviewer", "john")
	require.NoError(t, err)
	assert.JSONEq(t, `{"reviewers":["jane","john"],"team_reviewers":["plc-team"]}`, requested)
	assert.Equal(t, 1, countRequests(reg, "POST", reviewersPath))
	assert.Equal(t, createdOutput, stdout.String())
}

func TestNewCmdCreate_ReviewerTeamsOnly(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("POST", pullsPath), httpmock.StringResponse(http.StatusCreated, createdPR))
	var requested string
	reg.Register(httpmock.REST("POST", reviewersPath), recordBody(&requested, httpmock.StringResponse(http.StatusCreated, `[]`)))

	f, _ := metadataFactory(t, reg)
	err := runCreate(t, f, "--title", "feat: x", "--head", "feature/x", "-r", "my-org/plc-team")
	require.NoError(t, err)
	assert.JSONEq(t, `{"team_reviewers":["plc-team"]}`, requested)
}

func TestNewCmdCreate_NoReviewerSendsNoRequest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("POST", pullsPath), httpmock.StringResponse(http.StatusCreated, createdPR))

	f, _ := metadataFactory(t, reg)
	err := runCreate(t, f, "--title", "feat: x", "--head", "feature/x")
	require.NoError(t, err)
	assert.Len(t, reg.Requests, 1)
}

func TestCreateRun_ReviewerErrors(t *testing.T) {
	tests := []struct {
		name      string
		responder httpmock.Responder
		wantErr   string
	}{
		{
			name:      "non-201",
			responder: httpmock.StringResponse(http.StatusUnprocessableEntity, `{"message":"user does not exist"}`),
			wantErr:   `failed to request reviewers (HTTP 422): {"message":"user does not exist"}`,
		},
		{
			name: "transport error",
			responder: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("boom")
			},
			wantErr: "connecting to app.copia.io: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("POST", pullsPath), httpmock.StringResponse(http.StatusCreated, createdPR))
			reg.Register(httpmock.REST("POST", reviewersPath), tt.responder)

			ios, _, stdout, _ := iostreams.Test()
			err := CreateRun(&CreateOptions{
				IO:         ios,
				HTTPClient: &http.Client{Transport: reg},
				Host:       "app.copia.io",
				Token:      "test-token",
				Owner:      "my-org",
				Repo:       "my-repo",
				Title:      "feat: x",
				Base:       "main",
				Head:       "feature/x",
				Reviewers:  []string{"ghost"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			// The PR exists, so its URL is printed before the error.
			assert.Equal(t, createdOutput, stdout.String())
		})
	}
}

func TestNewCmdCreate_MetadataFlags(t *testing.T) {
	cmd := NewCmdCreate(&cmdutil.Factory{})
	for name, short := range map[string]string{"assignee": "a", "label": "l", "milestone": "m", "reviewer": "r"} {
		fl := cmd.Flags().Lookup(name)
		require.NotNil(t, fl, name)
		assert.Equal(t, short, fl.Shorthand, name)
	}
}
