package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// runRepoTime returns the creation time in the name of a run repository and
// false for any name runRepoName does not produce: another prefix, an empty
// or signed time, upper case or any character outside 0-9 and a-z.
func runRepoTime(name string) (time.Time, bool) {
	enc, ok := strings.CutPrefix(name, runRepoPrefix)
	if !ok || enc == "" {
		return time.Time{}, false
	}
	for _, c := range enc {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') {
			return time.Time{}, false
		}
	}
	sec, err := strconv.ParseInt(enc, 36, 64)
	if err != nil || sec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// deleteRunRepo deletes the repository owner/name and refuses, without a
// request, any name that is not a run repository's. It is the only delete
// of a repository in the integration tests.
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
	status, _, err := api(http.MethodPost, "/orgs/"+url.PathEscape(owner)+"/repos", map[string]any{
		"name":        name,
		"description": "Created by an integration test run of copia-cli; deleted at its end.",
		"private":     true,
		"auto_init":   true,
	})
	if err != nil {
		return "", fmt.Errorf("create run repository: %w", err)
	}
	if status != http.StatusCreated {
		return "", fmt.Errorf("create run repository: status %d, want %d", status, http.StatusCreated)
	}
	return name, nil
}

// tearDownRunRepo deletes the run repository owner/name and fails unless
// the API then answers 404 for it.
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
