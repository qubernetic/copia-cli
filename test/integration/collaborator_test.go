//go:build integration

package integration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
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
	require.True(t, err == nil, "repo collaborator add with read permission failed")
	assertOutputContains(t, out.String(), " with read permission\n")

	// The add took effect: Gitea answers 204 for a collaborator.
	repoPath := "/repos/" + env.Owner + "/" + env.Repo
	status, _ = apiCall(t, env, "is collaborator", "GET", repoPath+"/collaborators/"+me.Login, nil)
	assertStatusCode(t, status, http.StatusNoContent)

	// The permission route repo collaborator list reads for each
	// collaborator. Its answer is checked here first, by status and body
	// shape only, so a refusal says why without the login.
	status, body = apiCall(t, env, "collaborator permission", "GET", repoPath+"/collaborators/"+me.Login+"/permission", nil)
	require.False(t, status == http.StatusNotFound && strings.HasPrefix(string(body), "404 page not found"), "Copia does not register GET /repos/{owner}/{repo}/collaborators/{login}/permission (plain-text 404)")
	require.False(t, status == http.StatusNotFound && json.Valid(body), "GET /repos/{owner}/{repo}/collaborators/{login}/permission answers a Gitea JSON 404")
	assertStatusCode(t, status, http.StatusOK)

	// The permission of the account, selected by its login in jq, so the
	// output never holds the login. The error names the login, so only
	// whether there is one is checked.
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
	require.True(t, err == nil, "repo collaborator list --json failed")
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
	require.True(t, err == nil, "repo collaborator list failed")
	assertOutputContains(t, out.String(), "\tread\n")
}

// TestRepoCollaboratorAdd_UnknownUser adds a login no account has.
func TestRepoCollaboratorAdd_UnknownUser(t *testing.T) {
	env := loadTestEnv(t)

	// A random login, which no account has.
	b := make([]byte, 8)
	_, err := rand.Read(b)
	require.NoError(t, err, "random login")
	login := "it-nobody-" + hex.EncodeToString(b)

	ios, out, _ := testIO()
	err = collaboratoradd.AddRun(&collaboratoradd.AddOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: login, Permission: "read",
	})
	require.ErrorContains(t, err, "user "+login+" not found", "repo collaborator add of an unknown user")
	assertOutputEmpty(t, out.String())
}

// jqString quotes s as a jq string literal.
func jqString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
