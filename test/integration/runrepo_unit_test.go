package integration

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests carry no build tag: they run with the unit tests and need no
// Copia account, so a change that lets the delete path reach another
// repository fails before any integration run.

// recordingAPI is an apiFunc that answers every request with status and
// records the requests it got as "METHOD path".
func recordingAPI(status int, body string) (apiFunc, *[]string) {
	var calls []string
	return func(method, path string, _ any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		return status, []byte(body), nil
	}, &calls
}

func TestDeleteRunRepo_RefusesOtherNames(t *testing.T) {
	for _, name := range []string{
		"",
		"copia-cli-integration",
		"it-run-",
		"it-run",
		"it-runs-1",
		"IT-RUN-t3m5k0",
		"x-it-run-t3m5k0",
		"it-run-T3M5K0",
		"it-run--5",
		"it-run-+5",
		"it-run-t3m5k0-x",
		"it-run-t3m5k0/../other",
		"it-run-t3m_5k0",
		"it-run-0",
		"it-run-zzzzzzzzzzzzzzzzzzzz",
	} {
		api, calls := recordingAPI(204, "")
		err := deleteRunRepo(api, "owner", name)
		require.Error(t, err)
		require.True(t, len(*calls) == 0, "the delete path sent a request for a name it must refuse")
	}
}

func TestDeleteRunRepo_DeletesRunRepository(t *testing.T) {
	name := runRepoName(time.Unix(1_800_000_000, 0))
	require.True(t, name == "it-run-tro8w0", "runRepoName is it-run- and the base36 unix time")

	api, calls := recordingAPI(204, "")
	require.NoError(t, deleteRunRepo(api, "owner", name))
	require.True(t, len(*calls) == 1 && (*calls)[0] == "DELETE /repos/owner/it-run-tro8w0", "the delete path sent no single DELETE of the run repository")

	api, _ = recordingAPI(403, "")
	require.Error(t, deleteRunRepo(api, "owner", name))
}

func TestDeleteStaleRunRepos(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	stale := runRepoName(now.Add(-2*time.Hour - time.Second))
	fresh := runRepoName(now.Add(-2 * time.Hour))
	// Two pages of 50: the first holds the stale run repository, the second
	// the rest, so the sweep has to read past the first page.
	var first []map[string]string
	for i := 0; i < 49; i++ {
		first = append(first, map[string]string{"name": fmt.Sprintf("other-%d", i)})
	}
	first = append(first, map[string]string{"name": stale})
	second := []map[string]string{
		{"name": fresh},
		{"name": "copia-cli-integration"},
		{"name": "it-run-T3M5K0"},
		{"name": "x" + stale},
	}
	var calls []string
	api := func(method, path string, _ any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		var page any = []map[string]string{}
		switch path {
		case "/orgs/owner/repos?page=1&limit=50":
			page = first
		case "/orgs/owner/repos?page=2&limit=50":
			page = second
		}
		if method == "DELETE" {
			return 204, nil, nil
		}
		data, _ := json.Marshal(page)
		return 200, data, nil
	}

	require.NoError(t, deleteStaleRunRepos(api, "owner", now))
	require.True(t, slices.Equal(calls, []string{
		"GET /orgs/owner/repos?page=1&limit=50",
		"GET /orgs/owner/repos?page=2&limit=50",
		"DELETE /repos/owner/" + stale,
	}), "the sweep deletes exactly the run repository older than two hours")

	failing := func(method, path string, _ any) (int, []byte, error) { return 500, nil, nil }
	require.Error(t, deleteStaleRunRepos(failing, "owner", now))
}

func TestSetUpRunRepo(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var calls []string
	var created map[string]any
	api := func(method, path string, payload any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		if method == "POST" {
			data, _ := json.Marshal(payload)
			_ = json.Unmarshal(data, &created)
			return 201, nil, nil
		}
		return 200, []byte("[]"), nil
	}
	name, err := setUpRunRepo(api, "owner", now)
	require.NoError(t, err)
	require.True(t, name == "it-run-tro8w0", "the run repository is named after now")
	require.True(t, slices.Equal(calls, []string{
		"GET /orgs/owner/repos?page=1&limit=50",
		"POST /orgs/owner/repos",
	}), "set up sweeps the stale run repositories, then creates the run repository")
	require.True(t, created["name"] == name && created["private"] == true && created["auto_init"] == true,
		"the run repository is private and has an initial commit")

	api = func(method, path string, payload any) (int, []byte, error) {
		if method == "POST" {
			return 403, nil, nil
		}
		return 200, []byte("[]"), nil
	}
	_, err = setUpRunRepo(api, "owner", now)
	require.ErrorContains(t, err, "status 403")
}

func TestTearDownRunRepo(t *testing.T) {
	name := runRepoName(time.Unix(1_800_000_000, 0))
	for _, tt := range []struct {
		after int
		ok    bool
	}{{404, true}, {200, false}} {
		var calls []string
		api := func(method, path string, _ any) (int, []byte, error) {
			calls = append(calls, method+" "+path)
			if method == "DELETE" {
				return 204, nil, nil
			}
			return tt.after, nil, nil
		}
		err := tearDownRunRepo(api, "owner", name)
		require.True(t, (err == nil) == tt.ok, "tear down fails exactly when the repository is still there")
		require.True(t, slices.Equal(calls, []string{
			"DELETE /repos/owner/it-run-tro8w0",
			"GET /repos/owner/it-run-tro8w0",
		}), "tear down deletes the run repository and reads it back")
	}
}
