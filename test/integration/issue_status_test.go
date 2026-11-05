//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	issueclose "github.com/qubernetic/copia-cli/pkg/cmd/issue/close"
	issuecreate "github.com/qubernetic/copia-cli/pkg/cmd/issue/create"
	issuestatus "github.com/qubernetic/copia-cli/pkg/cmd/issue/status"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func TestIssue_Status(t *testing.T) {
	env := loadTestEnv(t)

	// An issue the test account opens and assigns to itself is the newest
	// of the assigned and the opened section.
	createIO, createOut, _ := testIO()
	err := issuecreate.CreateRun(&issuecreate.CreateOptions{
		IO: createIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Title:     "[integration-test] issue status",
		Body:      "Created by the issue status integration test.",
		Assignees: []string{"@me"},
	})
	require.NoError(t, err, "create issue")
	var number int64
	_, _ = fmt.Sscanf(createOut.String(), "Created issue #%d", &number)
	require.Greater(t, number, int64(0), "failed to parse issue number")
	defer func() {
		closeIO, _, _ := testIO()
		_ = issueclose.CloseRun(&issueclose.CloseOptions{
			IO: closeIO, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: number,
		})
	}()

	statusOpts := func() (*issuestatus.StatusOptions, func() string) {
		ios, out, _ := testIO()
		return &issuestatus.StatusOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Now: time.Now,
		}, out.String
	}

	opts, out := statusOpts()
	require.NoError(t, issuestatus.StatusRun(opts), "issue status")
	assertOutputContains(t, out(), "Issues assigned to you")
	assertOutputContains(t, out(), "Issues mentioning you")
	assertOutputContains(t, out(), "Issues opened by you")
	assertOutputContains(t, out(), "[integration-test] issue status")

	opts, out = statusOpts()
	opts.JSON = cmdutil.JSONFlags{Fields: []string{"number"}}
	require.NoError(t, issuestatus.StatusRun(opts), "issue status --json")
	var sections map[string][]struct {
		Number int64 `json:"number"`
	}
	require.NoError(t, json.Unmarshal([]byte(out()), &sections), "decode issue status --json")
	has := func(key string) bool {
		return slices.ContainsFunc(sections[key], func(i struct {
			Number int64 `json:"number"`
		}) bool {
			return i.Number == number
		})
	}
	require.True(t, has("assigned"), "the new issue is in assigned")
	require.True(t, has("createdBy"), "the new issue is in createdBy")
	_, ok := sections["mentioned"]
	require.True(t, ok, "the output has a mentioned section")

	// Every issue of the opened section is one of the test account's: Copia
	// ignores created_by, so the author check on the client is what keeps
	// issues of others out.
	status, body := apiCall(t, env, "current user", "GET", "/user", nil)
	assertStatusCode(t, status, http.StatusOK)
	var me struct {
		Login string `json:"login"`
	}
	require.NoError(t, json.Unmarshal(body, &me), "decode current user")
	opts, out = statusOpts()
	opts.JSON = cmdutil.JSONFlags{Fields: []string{"user"}}
	require.NoError(t, issuestatus.StatusRun(opts), "issue status --json user")
	var authors map[string][]struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal([]byte(out()), &authors), "decode issue status --json user")
	for _, i := range authors["createdBy"] {
		require.True(t, i.User.Login == me.Login, "an issue of createdBy has another author")
	}
}
