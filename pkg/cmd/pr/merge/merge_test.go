package merge

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

func TestMergeRun_Success(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
		httpmock.StringResponse(http.StatusOK, ``),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &MergeOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Method:     "merge",
	}

	err := MergeRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Merged PR #7")
}

func TestMergeRun_Squash(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
		httpmock.StringResponse(http.StatusOK, ``),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &MergeOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Method:     "squash",
	}

	err := MergeRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Merged PR #7")
}

func TestMergeRun_WithDeleteBranch(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
		httpmock.StringResponse(http.StatusOK, ``),
	)

	ios, _, stdout, _ := iostreams.Test()

	opts := &MergeOptions{
		IO:           ios,
		HTTPClient:   &http.Client{Transport: reg},
		Host:         "app.copia.io",
		Token:        "test-token",
		Owner:        "my-org",
		Repo:         "my-repo",
		Number:       7,
		Method:       "merge",
		DeleteBranch: true,
	}

	err := MergeRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Merged PR #7")
}

func requestJSON(t *testing.T, reg *httpmock.Registry) map[string]any {
	t.Helper()
	require.Len(t, reg.Requests, 1)
	raw, err := io.ReadAll(reg.Requests[0].Body)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}

func TestMergeRun_Body(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
		httpmock.StringResponse(http.StatusOK, ``),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &MergeOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Method:     "squash",
		Body:       "Reviewed on the test rig",
	}

	err := MergeRun(opts)
	require.NoError(t, err)
	got := requestJSON(t, reg)
	assert.Equal(t, "Reviewed on the test rig", got["MergeMessageField"])
	assert.Equal(t, "squash", got["Do"])
}

func TestMergeRun_NoBody_OmitsMergeMessageField(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
		httpmock.StringResponse(http.StatusOK, ``),
	)

	ios, _, _, _ := iostreams.Test()

	opts := &MergeOptions{
		IO:         ios,
		HTTPClient: &http.Client{Transport: reg},
		Host:       "app.copia.io",
		Token:      "test-token",
		Owner:      "my-org",
		Repo:       "my-repo",
		Number:     7,
		Method:     "merge",
	}

	err := MergeRun(opts)
	require.NoError(t, err)
	got := requestJSON(t, reg)
	assert.NotContains(t, got, "MergeMessageField")
}

func TestNewCmdMerge_BodyFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantDo     string
		wantDelete bool
	}{
		{name: "long flag", args: []string{"7", "--body", "text"}, wantDo: "merge"},
		{name: "short flag", args: []string{"7", "-b", "text"}, wantDo: "merge"},
		{name: "with squash", args: []string{"7", "--squash", "--body", "text"}, wantDo: "squash"},
		{name: "with rebase and delete branch", args: []string{"7", "--rebase", "-b", "text", "--delete-branch"}, wantDo: "rebase", wantDelete: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			httpmock.ReplaceDefaultTransport(t, reg)
			reg.Register(
				httpmock.REST("POST", "/api/v1/repos/my-org/my-repo/pulls/7/merge"),
				httpmock.StringResponse(http.StatusOK, ``),
			)

			ios, _, stdout, _ := iostreams.Test()
			cmd := NewCmdMerge(&cmdutil.Factory{IOStreams: ios, Host: "app.copia.io", Token: "test-token", Repo: "my-org/my-repo"})
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Contains(t, stdout.String(), "Merged PR #7")
			got := requestJSON(t, reg)
			assert.Equal(t, "text", got["MergeMessageField"])
			assert.Equal(t, tt.wantDo, got["Do"])
			if tt.wantDelete {
				assert.Equal(t, true, got["delete_branch_after_merge"])
			} else {
				assert.NotContains(t, got, "delete_branch_after_merge")
			}
		})
	}
}
