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
	collaboratorremove "github.com/qubernetic/copia-cli/pkg/cmd/repo/collaborator/remove"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// TestRepoCollaborator_AddAndList adds the test account itself to the run
// repository with read permission and finds it in repo collaborator list,
// which shows no permission: Copia does not register Gitea's route for one
// collaborator's permission. The collaborator goes with the run repository
// when TestMain deletes it.
func TestRepoCollaborator_AddAndList(t *testing.T) {
	env := loadTestEnv(t)

	me := currentLogin(t, env)

	ios, out, _ := testIO()
	err := collaboratoradd.AddRun(&collaboratoradd.AddOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: me, Permission: "read",
	})
	require.True(t, err == nil, "repo collaborator add with read permission failed")
	assertOutputContains(t, out.String(), " with read permission\n")

	// The add took effect: Gitea answers 204 for a collaborator.
	status, _ := apiCall(t, env, "is collaborator", "GET", "/repos/"+env.Owner+"/"+env.Repo+"/collaborators/"+me, nil)
	assertStatusCode(t, status, http.StatusNoContent)

	// The account is listed once, selected by its login in jq, so the
	// output never holds the login. The error names the login, so only
	// whether there is one is checked.
	ios, out, _ = testIO()
	err = collaboratorlist.ListRun(&collaboratorlist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
		JSON: cmdutil.JSONFlags{
			Fields: []string{"login"},
			JQ:     `[.[] | select(.login == ` + jqString(me) + `)] | length`,
		},
	})
	require.True(t, err == nil, "repo collaborator list --json failed")
	assertOutputEqual(t, out.String(), "1\n")

	// Without a terminal: one line per collaborator, the login, a tab and
	// the name. Checked with require.True, so the login is never printed.
	ios, out, _ = testIO()
	err = collaboratorlist.ListRun(&collaboratorlist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
	})
	require.True(t, err == nil, "repo collaborator list failed")
	require.True(t, strings.Contains("\n"+out.String(), "\n"+me+"\t"), "repo collaborator list does not show the account")
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

// TestRepoCollaborator_Remove adds the test account itself to the run
// repository, removes it with repo collaborator remove and no longer finds
// it in repo collaborator list. A second remove fails, since the account is
// no collaborator any more.
func TestRepoCollaborator_Remove(t *testing.T) {
	env := loadTestEnv(t)
	me := currentLogin(t, env)

	ios, _, _ := testIO()
	err := collaboratoradd.AddRun(&collaboratoradd.AddOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: me, Permission: "read",
	})
	require.True(t, err == nil, "repo collaborator add failed")

	// The success line names the login and the repository, so it is
	// compared as a secret.
	ios, out, _ := testIO()
	err = collaboratorremove.RemoveRun(&collaboratorremove.RemoveOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: me,
	})
	require.True(t, err == nil, "repo collaborator remove failed")
	assertOutputEqualSecret(t, out.String(), "Removed "+me+" from "+env.Owner+"/"+env.Repo+"\n")

	// The remove took effect: Gitea answers 404 for a non-collaborator.
	status, _ := apiCall(t, env, "is collaborator", "GET", "/repos/"+env.Owner+"/"+env.Repo+"/collaborators/"+me, nil)
	assertStatusCode(t, status, http.StatusNotFound)

	ios, out, _ = testIO()
	err = collaboratorlist.ListRun(&collaboratorlist.ListOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
		JSON: cmdutil.JSONFlags{
			Fields: []string{"login"},
			JQ:     `[.[] | select(.login == ` + jqString(me) + `)] | length`,
		},
	})
	require.True(t, err == nil, "repo collaborator list --json failed")
	assertOutputEqual(t, out.String(), "0\n")

	// The error names the login, so only its fixed part is checked.
	ios, out, _ = testIO()
	err = collaboratorremove.RemoveRun(&collaboratorremove.RemoveOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
		Login: me,
	})
	require.True(t, err != nil && strings.Contains(err.Error(), " is not a collaborator of "), "second repo collaborator remove did not fail as a non-collaborator")
	assertOutputEmpty(t, out.String())
}

// currentLogin returns the login of the test account.
func currentLogin(t *testing.T, env testEnv) string {
	t.Helper()
	status, body := apiCall(t, env, "current user", "GET", "/user", nil)
	assertStatusCode(t, status, http.StatusOK)
	var me struct {
		Login string `json:"login"`
	}
	require.NoError(t, json.Unmarshal(body, &me), "decode current user")
	require.True(t, me.Login != "", "current user has a login")
	return me.Login
}
