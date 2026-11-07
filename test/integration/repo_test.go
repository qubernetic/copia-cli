//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoarchive "github.com/qubernetic/copia-cli/pkg/cmd/repo/archive"
	repoclone "github.com/qubernetic/copia-cli/pkg/cmd/repo/clone"
	repoedit "github.com/qubernetic/copia-cli/pkg/cmd/repo/edit"
	repofork "github.com/qubernetic/copia-cli/pkg/cmd/repo/fork"
	repolist "github.com/qubernetic/copia-cli/pkg/cmd/repo/list"
	reporename "github.com/qubernetic/copia-cli/pkg/cmd/repo/rename"
	reposync "github.com/qubernetic/copia-cli/pkg/cmd/repo/sync"
	repoview "github.com/qubernetic/copia-cli/pkg/cmd/repo/view"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func TestRepoList_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &repolist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      5,
	}

	err := repolist.ListRun(opts)
	require.NoError(t, err)
	assertOutputNotEmpty(t, stdout.String())
}

func TestRepoList_JSON(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &repolist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Limit:      2,
		JSON:       cmdutil.JSONFlags{Fields: []string{"full_name"}},
	}

	err := repolist.ListRun(opts)
	require.NoError(t, err)
	assertOutputContains(t, stdout.String(), "full_name")
}

func TestRepoView_Run(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &repoview.ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
	}

	err := repoview.ViewRun(opts)
	require.NoError(t, err)
	output := stdout.String()
	assertOutputContains(t, output, env.Owner+"/"+env.Repo)
	assertOutputContains(t, output, "Default branch")
}

func TestRepoView_NotFound(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &repoview.ViewOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      "nonexistent-org-xyz",
		Repo:       "nonexistent-repo-xyz",
	}

	err := repoview.ViewRun(opts)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

// TestRepoEdit edits a scratch repository, since it changes the visibility:
// the description and topics, then public (refused by Copia) and private.
func TestRepoEdit(t *testing.T) {
	env := loadTestEnv(t)
	scratch := newScratchRepo(t, env, "edit")

	edit := func(opts repoedit.EditOptions) {
		t.Helper()
		ios, stdout, stderr := testIO()
		opts.IO, opts.HTTPClient = ios, testHTTPClient()
		opts.Host, opts.Token, opts.Owner, opts.Repo = scratch.Host, scratch.Token, scratch.Owner, scratch.Repo
		require.NoError(t, repoedit.EditRun(&opts), "edit the scratch repository")
		assertOutputEqual(t, stdout.String(), "Edited repository "+scratch.Owner+"/"+scratch.Repo+"\n")
		assertOutputEmpty(t, stderr.String())
	}

	description := "Edited by the integration test"
	edit(repoedit.EditOptions{
		Edits:     repoedit.EditRepositoryInput{Description: &description},
		AddTopics: []string{"copia-cli-it", "plc"},
	})
	desc, private := repoSettings(t, scratch)
	assert.True(t, desc == description, "the description was not changed")
	assert.True(t, private, "a description edit changed the visibility")
	assert.True(t, slices.Equal(repoTopics(t, scratch), []string{"copia-cli-it", "plc"}), "the topics were not added")

	edit(repoedit.EditOptions{RemoveTopics: []string{"plc"}})
	assert.True(t, slices.Equal(repoTopics(t, scratch), []string{"copia-cli-it"}), "the topic was not removed")

	// app.copia.io keeps repositories private (Gitea FORCE_PRIVATE): the
	// switch to public is refused with 403 and changes nothing.
	public, priv := "public", "private"
	ios, stdout, _ := testIO()
	err := repoedit.EditRun(&repoedit.EditOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: scratch.Host, Token: scratch.Token, Owner: scratch.Owner, Repo: scratch.Repo,
		Visibility: &public,
	})
	require.ErrorContains(t, err, "failed to edit repository (HTTP 403): the server may not allow public repositories", "the switch to public was not refused as expected")
	assertOutputEmpty(t, stdout.String())
	_, private = repoSettings(t, scratch)
	assert.True(t, private, "a refused switch to public changed the visibility")

	edit(repoedit.EditOptions{Visibility: &priv})
	desc, private = repoSettings(t, scratch)
	assert.True(t, private, "the repository is not private")
	assert.True(t, desc == description, "a visibility edit changed the description")
}

// TestRepoRename renames a scratch repository to another scratch name; the
// run deletes it under the new name.
func TestRepoRename(t *testing.T) {
	env := loadTestEnv(t)
	scratch := newScratchRepo(t, env, "rename")
	renamed := scratch
	renamed.Repo = env.Repo + "-renamed"

	ios, stdout, stderr := testIO()
	err := reporename.RenameRun(&reporename.RenameOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: scratch.Host, Token: scratch.Token, Owner: scratch.Owner, Repo: scratch.Repo,
		NewName: renamed.Repo, Confirmed: true,
	})
	// Whatever RenameRun reports, the repository the server has under the
	// new name is the one the run must delete.
	status, _ := apiCall(t, renamed, "get renamed repository", "GET", "/repos/"+renamed.Owner+"/"+renamed.Repo, nil)
	if status == http.StatusOK {
		renameScratchRepo(scratch.Repo, renamed.Repo)
	}
	require.NoError(t, err, "rename the scratch repository")
	assertOutputEqual(t, stdout.String(), "Renamed repository "+renamed.Owner+"/"+renamed.Repo+"\n")
	assertOutputEmpty(t, stderr.String())
	require.True(t, status == http.StatusOK, "the repository is not found under its new name")
}

// TestRepoArchive archives and unarchives a scratch repository; repo view
// --json archived shows the state after each, and a second run of either
// only warns.
func TestRepoArchive(t *testing.T) {
	env := loadTestEnv(t)
	scratch := newScratchRepo(t, env, "archive")
	fullName := scratch.Owner + "/" + scratch.Repo

	setArchived := func(archived bool) (string, string) {
		t.Helper()
		ios, stdout, stderr := testIO()
		err := repoarchive.Run(&repoarchive.Options{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: scratch.Host, Token: scratch.Token, Owner: scratch.Owner, Repo: scratch.Repo,
			Confirmed: true, Archived: archived,
		})
		require.NoError(t, err, "set the archived state of the scratch repository")
		return stdout.String(), stderr.String()
	}
	viewArchived := func() string {
		t.Helper()
		ios, stdout, _ := testIO()
		err := repoview.ViewRun(&repoview.ViewOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: scratch.Host, Token: scratch.Token, Owner: scratch.Owner, Repo: scratch.Repo,
			JSON: cmdutil.JSONFlags{Fields: []string{"archived"}},
		})
		require.NoError(t, err, "view the scratch repository")
		return stdout.String()
	}

	stdout, stderr := setArchived(true)
	assertOutputEqual(t, stdout, "Archived repository "+fullName+"\n")
	assertOutputEmpty(t, stderr)
	assertOutputEqual(t, viewArchived(), "{\n  \"archived\": true\n}\n")

	stdout, stderr = setArchived(true)
	assertOutputEmpty(t, stdout)
	assertOutputEqual(t, stderr, "! Repository "+fullName+" is already archived\n")

	stdout, stderr = setArchived(false)
	assertOutputEqual(t, stdout, "Unarchived repository "+fullName+"\n")
	assertOutputEmpty(t, stderr)
	assertOutputEqual(t, viewArchived(), "{\n  \"archived\": false\n}\n")

	stdout, stderr = setArchived(false)
	assertOutputEmpty(t, stdout)
	assertOutputEqual(t, stderr, "! Repository "+fullName+" is not archived\n")
}

// TestRepoFork checks that repo fork of the run repository reports that
// Copia has no forks route, and that no fork appears under the test account.
func TestRepoFork(t *testing.T) {
	env := loadTestEnv(t)

	ios, stdout, stderr := testIO()
	err := repofork.ForkRun(&repofork.ForkOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
	})
	require.ErrorContains(t, err, "Copia does not offer forking through its API", "repo fork of the run repository")
	assertOutputEmpty(t, stdout.String())
	assertOutputEmpty(t, stderr.String())

	status, body := apiCall(t, env, "current user", "GET", "/user", nil)
	assertStatusCode(t, status, http.StatusOK)
	var me struct {
		Login string `json:"login"`
	}
	require.NoError(t, json.Unmarshal(body, &me), "decode current user")
	status, _ = apiCall(t, env, "get fork", "GET", "/repos/"+me.Login+"/"+env.Repo, nil)
	assertStatusCode(t, status, http.StatusNotFound)
}

// TestRepoSync clones the run repository, checks out a branch of its own,
// adds a commit to that branch through the API and syncs the clone with
// repo sync, which fetches with the token over https. The clone's branch
// then points at the server's head. The remote mode of the run repository,
// which is not a fork, fails: app.copia.io has no merge-upstream route.
func TestRepoSync(t *testing.T) {
	env := loadTestEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	repoPath := "/repos/" + env.Owner + "/" + env.Repo
	branch := "it-sync-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	status, _ := apiCall(t, env, "create branch", "POST", repoPath+"/branches", map[string]string{"new_branch_name": branch})
	assertStatusCode(t, status, http.StatusCreated)
	t.Cleanup(func() {
		status, _ := apiCall(t, env, "delete branch", "DELETE", repoPath+"/branches/"+branch, nil)
		assertStatusCode(t, status, http.StatusNoContent)
	})

	work := filepath.Join(t.TempDir(), "work")
	cloneIO, _, _ := testIO()
	err := repoclone.CloneRun(&repoclone.CloneOptions{
		IO: cloneIO, Host: env.Host, Token: env.Token,
		Repo: env.Owner + "/" + env.Repo, Dir: work,
	})
	require.True(t, err == nil, "repo clone of the run repository failed")
	t.Chdir(work)
	_, err = exec.Command("git", "checkout", "-q", branch).CombinedOutput()
	require.True(t, err == nil, "git checkout of the sync branch failed")

	commitFile(t, env, branch, "integration-test/"+branch+".txt")

	ios, stdout, stderr := testIO()
	err = reposync.SyncRun(&reposync.SyncOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		BaseRepo: func() (string, string, error) { return env.Owner, env.Repo, nil },
		Branch:   branch,
	})
	require.True(t, err == nil, "repo sync of the clone failed")
	assertOutputContains(t, stdout.String(), `Synced the "`+branch+`" branch from "`)
	assertOutputEmpty(t, stderr.String())

	status, body := apiCall(t, env, "get branch", "GET", repoPath+"/branches/"+branch, nil)
	assertStatusCode(t, status, http.StatusOK)
	var remote struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	require.NoError(t, json.Unmarshal(body, &remote), "decode the branch")
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	require.True(t, err == nil, "git rev-parse HEAD failed")
	require.True(t, remote.Commit.ID != "" && strings.TrimSpace(string(out)) == remote.Commit.ID, "the synced branch does not point at the server's head")

	remoteIO, remoteOut, _ := testIO()
	err = reposync.SyncRun(&reposync.SyncOptions{
		IO: remoteIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		DestArg: env.Owner + "/" + env.Repo,
		Branch:  branch,
	})
	require.ErrorContains(t, err, "Copia does not offer fork sync through its API", "repo sync of the run repository")
	assertOutputEmpty(t, remoteOut.String())
}

// repoSettings returns the description and the private flag of the
// repository of env.
func repoSettings(t *testing.T, env testEnv) (string, bool) {
	t.Helper()
	status, data := apiCall(t, env, "get repository", "GET", "/repos/"+env.Owner+"/"+env.Repo, nil)
	require.True(t, status == http.StatusOK, "get repository did not answer 200")
	var repo struct {
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}
	require.NoError(t, json.Unmarshal(data, &repo), "decode the repository")
	return repo.Description, repo.Private
}

// repoTopics returns the topics of the repository of env, sorted.
func repoTopics(t *testing.T, env testEnv) []string {
	t.Helper()
	status, data := apiCall(t, env, "get topics", "GET", "/repos/"+env.Owner+"/"+env.Repo+"/topics", nil)
	require.True(t, status == http.StatusOK, "get topics did not answer 200")
	var topics struct {
		Topics []string `json:"topics"`
	}
	require.NoError(t, json.Unmarshal(data, &topics), "decode the topics")
	slices.Sort(topics.Topics)
	return topics.Topics
}
