//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	collaboratoradd "github.com/qubernetic/copia-cli/pkg/cmd/repo/collaborator/add"
	collaboratorlist "github.com/qubernetic/copia-cli/pkg/cmd/repo/collaborator/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// TestRepoCollaborator_AddAndList adds the test account itself to the run
// repository with read permission and finds it in repo collaborator list.
// The collaborator goes with the run repository when TestMain deletes it.
func TestRepoCollaborator_AddAndList(t *testing.T) {
	env := loadTestEnv(t)

	status, body := apiCall(t, env, "current user", "GET", "/user", nil)
	assertStatusCode(t, status, http.StatusOK)
	var me struct {
		Login string `json:"login"`
	}
	require.NoError(t, json.Unmarshal(body, &me), "decode current user")
	require.True(t, me.Login != "", "current user has a login")

	ios, out, _ := testIO()
	err := collaboratoradd.AddRun(&collaboratoradd.AddOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: me.Login, Permission: "read",
	})
	require.NoError(t, err, "repo collaborator add with read permission")
	assertOutputContains(t, out.String(), " with read permission\n")

	// The permission of the account, selected by its login in jq, so the
	// output never holds the login.
	ios, out, _ = testIO()
	err = collaboratorlist.ListRun(&collaboratorlist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
		JSON: cmdutil.JSONFlags{
			Fields: []string{"login", "permission"},
			JQ:     `[.[] | select(.login == ` + jqString(me.Login) + `) | .permission] | join(",")`,
		},
	})
	require.NoError(t, err, "repo collaborator list --json failed")
	permission := out.String()
	require.False(t, permission == "\n", "repo collaborator list does not show the account")
	require.False(t, permission == "owner\n", "repo collaborator list shows owner permission, not read")
	require.False(t, permission == "admin\n", "repo collaborator list shows admin permission, not read")
	require.False(t, permission == "write\n", "repo collaborator list shows write permission, not read")
	assertOutputEqual(t, permission, "read\n")

	// Without a terminal: one line per collaborator, no error.
	ios, out, _ = testIO()
	err = collaboratorlist.ListRun(&collaboratorlist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
	})
	require.NoError(t, err, "repo collaborator list failed")
	assertOutputContains(t, out.String(), "\tread\n")
}

// TestRepoCollaboratorAdd_UnknownUser adds a login no account has.
func TestRepoCollaboratorAdd_UnknownUser(t *testing.T) {
	env := loadTestEnv(t)

	ios, out, _ := testIO()
	err := collaboratoradd.AddRun(&collaboratoradd.AddOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: runRepo.name + "-nobody", Permission: "read",
	})
	require.ErrorContains(t, err, "-nobody not found", "repo collaborator add of an unknown user")
	assertOutputEmpty(t, out.String())
}

// jqString quotes s as a jq string literal.
func jqString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
