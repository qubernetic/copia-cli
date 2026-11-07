package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The integration tests run in a private repository of COPIA_TEST_OWNER
// that the run creates and deletes, since Copia answers slower as a
// repository collects pull requests, issues and notifications that cannot be
// deleted one by one. This file carries no build tag, so its unit tests
// check the delete path with every go test run.

// runRepoPrefix starts the name of every run repository; the base36 unix
// time of its creation follows it.
const runRepoPrefix = "it-run-"

// apiFunc sends one request to the Copia API, path below /api/v1, and
// returns the status and the response body.
type apiFunc func(method, path string, payload any) (int, []byte, error)

// runRepoName is the name of the run repository created at now.
func runRepoName(now time.Time) string {
	return runRepoPrefix + strconv.FormatInt(now.Unix(), 36)
}

// runRepoTime returns the creation time in the name of a run repository or
// of one of its scratch repositories, and false for any name runRepoName and
// scratchRepoName do not produce: another prefix, an empty or signed time, an
// empty suffix, upper case or any character outside 0-9 and a-z but the one
// hyphen before the suffix.
func runRepoTime(name string) (time.Time, bool) {
	rest, ok := strings.CutPrefix(name, runRepoPrefix)
	if !ok {
		return time.Time{}, false
	}
	enc, suffix, scratch := strings.Cut(rest, "-")
	if !isBase36Digits(enc) || (scratch && !isBase36Digits(suffix)) {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(enc, 36, 64)
	if err != nil || sec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// isBase36Digits reports whether s is not empty and holds only 0-9 and a-z.
func isBase36Digits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

// deleteRunRepo deletes the repository owner/name and refuses, without a
// request, any name that is not a run or scratch repository's. It is the
// only delete of a repository in the integration tests.
func deleteRunRepo(api apiFunc, owner, name string) error {
	if _, ok := runRepoTime(name); !ok {
		return fmt.Errorf("refusing to delete a repository whose name does not start with %s and a time", runRepoPrefix)
	}
	status, _, err := api(http.MethodDelete, "/repos/"+url.PathEscape(owner)+"/"+name, nil)
	if err != nil {
		return fmt.Errorf("delete run repository: %w", err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("delete run repository: status %d, want %d", status, http.StatusNoContent)
	}
	return nil
}

// runRepoMaxAge is how long a run repository may stay: a run that hits the
// go test -timeout ends without its cleanup, so the next run past this age
// deletes it. It is well past the 30 minute timeout of the integration step.
const runRepoMaxAge = 2 * time.Hour

// deleteStaleRunRepos deletes every run repository of the organization owner
// created more than runRepoMaxAge before now. It reads all pages before the
// first delete, so no deletion shifts a page it has yet to read.
func deleteStaleRunRepos(api apiFunc, owner string, now time.Time) error {
	const limit = 50
	var stale []string
	for page := 1; ; page++ {
		status, data, err := api(http.MethodGet, fmt.Sprintf("/orgs/%s/repos?page=%d&limit=%d", url.PathEscape(owner), page, limit), nil)
		if err != nil {
			return fmt.Errorf("list repositories: %w", err)
		}
		if status != http.StatusOK {
			return fmt.Errorf("list repositories: status %d, want %d", status, http.StatusOK)
		}
		var repos []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &repos); err != nil {
			return fmt.Errorf("list repositories: undecodable response")
		}
		for _, r := range repos {
			if created, ok := runRepoTime(r.Name); ok && now.Sub(created) > runRepoMaxAge {
				stale = append(stale, r.Name)
			}
		}
		if len(repos) < limit {
			break
		}
	}
	for _, name := range stale {
		if err := deleteRunRepo(api, owner, name); err != nil {
			return err
		}
	}
	return nil
}

// setUpRunRepo deletes the stale run repositories of the organization owner
// and creates the private run repository of now with an initial commit, so
// it has a default branch. It returns the repository's name.
func setUpRunRepo(api apiFunc, owner string, now time.Time) (string, error) {
	if err := deleteStaleRunRepos(api, owner, now); err != nil {
		return "", err
	}
	name := runRepoName(now)
	if err := createRepo(api, owner, name, "run"); err != nil {
		return "", err
	}
	return name, nil
}

// createScratchRepo creates the private scratch repository of the run
// repository run with suffix, a test's own repository for what it cannot do
// in the shared one, and returns its name. It refuses, without a request, a
// name the delete path would refuse: suffix must be 0-9 and a-z only.
func createScratchRepo(api apiFunc, owner, run, suffix string) (string, error) {
	name := run + "-" + suffix
	if _, ok := runRepoTime(run); !ok || strings.Contains(run[len(runRepoPrefix):], "-") || !isBase36Digits(suffix) {
		return "", fmt.Errorf("refusing to create a scratch repository whose name is not a run repository's, a hyphen and 0-9 or a-z")
	}
	if err := createRepo(api, owner, name, "scratch"); err != nil {
		return "", err
	}
	return name, nil
}

// createRepo creates the private repository name of the organization owner
// with an initial commit, so it has a default branch. kind names it in
// errors, which give the status only.
func createRepo(api apiFunc, owner, name, kind string) error {
	status, _, err := api(http.MethodPost, "/orgs/"+url.PathEscape(owner)+"/repos", map[string]any{
		"name":        name,
		"description": "Created by an integration test run of copia-cli; deleted at its end.",
		"private":     true,
		"auto_init":   true,
	})
	if err != nil {
		return fmt.Errorf("create %s repository: %w", kind, err)
	}
	if status != http.StatusCreated {
		return fmt.Errorf("create %s repository: status %d, want %d", kind, status, http.StatusCreated)
	}
	return nil
}

// tearDownRun deletes the scratch repositories of the run, then the run
// repository, each with tearDownRunRepo. It goes on past a failure, so one
// repository left behind does not keep the others, and returns every error.
func tearDownRun(api apiFunc, owner, run string, scratch []string) error {
	var errs []error
	for _, name := range append(slices.Clone(scratch), run) {
		errs = append(errs, tearDownRunRepo(api, owner, name))
	}
	return errors.Join(errs...)
}

// tearDownRunRepo deletes the run or scratch repository owner/name and fails
// unless the API then answers 404 for it.
func tearDownRunRepo(api apiFunc, owner, name string) error {
	if err := deleteRunRepo(api, owner, name); err != nil {
		return err
	}
	status, _, err := api(http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+name, nil)
	if err != nil {
		return fmt.Errorf("read deleted run repository: %w", err)
	}
	if status != http.StatusNotFound {
		return fmt.Errorf("read deleted run repository: status %d, want %d", status, http.StatusNotFound)
	}
	return nil
}
