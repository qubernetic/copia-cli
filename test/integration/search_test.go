//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	searchissues "github.com/qubernetic/copia-cli/pkg/cmd/search/issues"
	searchprs "github.com/qubernetic/copia-cli/pkg/cmd/search/prs"
	searchrepos "github.com/qubernetic/copia-cli/pkg/cmd/search/repos"
)

func TestSearchRepos_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &searchrepos.SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Query:      "integration",
		Limit:      5,
	}

	err := searchrepos.SearchRun(opts)
	require.NoError(t, err)
	assertOutputContains(t, stdout.String(), "integration")
}

func TestSearchIssues_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &searchissues.SearchOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Query:      "lifecycle",
		Limit:      5,
	}

	err := searchissues.SearchRun(opts)
	require.NoError(t, err)
}

func TestSearchPRs_Run(t *testing.T) {
	env := loadTestEnv(t)
	token := "searchprs" + strconv.FormatInt(time.Now().UnixNano(), 36)
	pr := createFixturePR(t, env, "[integration-test] CLI search prs test "+token)

	status, data := apiCall(t, env, "read user", "GET", "/user", nil)
	assertStatusCode(t, status, http.StatusOK)
	var me struct {
		Login string `json:"login"`
	}
	require.NoError(t, json.Unmarshal(data, &me))

	// search runs the command with the fixture's token as the query and
	// prints how many results are the fixture pull request.
	search := func(set func(*searchprs.SearchOptions)) (string, error) {
		t.Helper()
		ios, out, _ := testIO()
		opts := &searchprs.SearchOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Query: token,
			Repo:  env.Owner + "/" + env.Repo,
			Limit: 10,
			JSON: cmdutil.JSONFlags{
				Fields: []string{"number", "state"},
				JQ:     fmt.Sprintf(`[.[] | select(.number == %d and .state == "open")] | length`, pr.Number),
			},
		}
		if set != nil {
			set(opts)
		}
		err := searchprs.SearchRun(opts)
		return out.String(), err
	}
	// eventually waits until search prints want, since Gitea indexes a new
	// or changed pull request in the background.
	eventually := func(want string, set func(*searchprs.SearchOptions)) {
		t.Helper()
		var out string
		for i := 0; i < 30; i++ {
			var err error
			out, err = search(set)
			require.NoError(t, err)
			if out == want {
				break
			}
			time.Sleep(time.Second)
		}
		assertOutputEqual(t, out, want)
	}

	eventually("1\n", nil)
	eventually("1\n", func(o *searchprs.SearchOptions) { o.State = "open"; o.Author = "@me" })
	eventually("1\n", func(o *searchprs.SearchOptions) { o.Author = me.Login; o.Owner = env.Owner })
	out, err := search(func(o *searchprs.SearchOptions) { o.State = "closed" })
	require.NoError(t, err)
	assertOutputEqual(t, out, "0\n")
	out, err = search(func(o *searchprs.SearchOptions) { o.Assignee = "@me" })
	require.NoError(t, err)
	assertOutputEqual(t, out, "0\n")

	// Gitea 1.26 answers created_by with an unknown user with HTTP 404;
	// app.copia.io ignores created_by (found by the integration job of
	// #376), so the command checks the author of each result.
	out, err = search(func(o *searchprs.SearchOptions) { o.Author = token + "-nobody" })
	if err != nil {
		require.ErrorContains(t, err, "no user named", "created_by with an unknown user")
	} else {
		assertOutputEqual(t, out, "0\n")
	}

	status, _ = apiCall(t, env, "assign pull request", "PATCH",
		fmt.Sprintf("/repos/%s/%s/issues/%d", env.Owner, env.Repo, pr.Number), map[string]any{"assignees": []string{me.Login}})
	assertStatusCode(t, status, http.StatusCreated)
	eventually("1\n", func(o *searchprs.SearchOptions) { o.Assignee = "@me" })
}
