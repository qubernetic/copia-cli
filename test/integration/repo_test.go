//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoarchive "github.com/qubernetic/copia-cli/pkg/cmd/repo/archive"
	repoedit "github.com/qubernetic/copia-cli/pkg/cmd/repo/edit"
	repofork "github.com/qubernetic/copia-cli/pkg/cmd/repo/fork"
	repolist "github.com/qubernetic/copia-cli/pkg/cmd/repo/list"
	reporename "github.com/qubernetic/copia-cli/pkg/cmd/repo/rename"
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

// TestRepoFork forks the run repository into the test owner under a
// scratch name with --clone: the clone has the fork as origin and the run
// repository as upstream, the default repository. A second fork only
// reports that the fork exists.
func TestRepoFork(t *testing.T) {
	env := loadTestEnv(t)
	fork := env
	fork.Repo = env.Repo + "-fork"
	work := t.TempDir()
	t.Chdir(work)

	runFork := func(clone bool) (string, string, error) {
		t.Helper()
		ios, stdout, stderr := testIO()
		err := repofork.ForkRun(&repofork.ForkOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token, Owner: env.Owner, Repo: env.Repo,
			Org: env.Owner, ForkName: fork.Repo, Clone: clone,
			RemoteName: "origin", Rename: true,
		})
		return stdout.String(), stderr.String(), err
	}

	stdout, _, err := runFork(true)
	// Whatever ForkRun reports, a fork the server has is the run's to delete.
	status, _ := apiCall(t, fork, "get fork", "GET", "/repos/"+fork.Owner+"/"+fork.Repo, nil)
	if status == http.StatusOK {
		addScratchRepo(fork.Repo)
	}
	require.NoError(t, err, "fork and clone the run repository")
	require.True(t, status == http.StatusOK, "the fork is not found")
	assertOutputContains(t, stdout, "/"+fork.Repo+"\n")

	dir := filepath.Join(work, fork.Repo)
	remotes, err := exec.Command("git", "-C", dir, "remote").Output()
	require.NoError(t, err, "list the remotes of the clone")
	assert.True(t, string(remotes) == "origin\nupstream\n", "the clone does not have exactly origin and upstream")
	resolved, err := exec.Command("git", "-C", dir, "config", "--get", "remote.upstream.copia-resolved").Output()
	require.NoError(t, err, "read the default remote of the clone")
	assert.True(t, string(resolved) == "base\n", "upstream is not the default repository")
	branches, err := exec.Command("git", "-C", dir, "branch", "-r").Output()
	require.NoError(t, err, "list the remote branches of the clone")
	assertOutputContains(t, string(branches), "upstream/")

	stdout, stderr, err := runFork(false)
	require.NoError(t, err, "fork the run repository again")
	assertOutputEmpty(t, stdout)
	assertOutputContains(t, stderr, "/"+fork.Repo+" already exists\n")
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
