package integration

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests carry no build tag: they run with the unit tests and need no
// Copia account, so a change that lets the GPG key delete path reach another
// key of the test account fails before any integration run.

const (
	testKeyID  = "7100E3D5D90A4D53"
	testMarker = "it-run-tro8w0-gk7100e3d5d90a4d53"
)

func TestGPGKeyMarker(t *testing.T) {
	marker, err := gpgKeyMarker("it-run-tro8w0", testKeyID)
	require.NoError(t, err)
	require.True(t, marker == testMarker, "the marker is the run repository's name, -gk and the lower-case key ID")

	for _, bad := range [][2]string{
		{"it-run-tro8w0", ""},
		{"it-run-tro8w0", "7100E3D5D90A4D5"},
		{"it-run-tro8w0", "7100E3D5D90A4D533"},
		{"it-run-tro8w0", "7100E3D5D90A4D5G"},
		{"it-run-tro8w0-x", testKeyID},
		{"other", testKeyID},
		{"", testKeyID},
	} {
		_, err := gpgKeyMarker(bad[0], bad[1])
		require.Error(t, err)
	}
}

func TestMarkerKeyID(t *testing.T) {
	keyID, created, ok := markerKeyID(testMarker)
	require.True(t, ok, "a marker is not read as one")
	require.True(t, keyID == testKeyID, "the marker's key ID is not the upper-case key ID")
	require.True(t, created.Equal(time.Unix(1_800_000_000, 0)), "the marker's time is not the run's")

	for _, name := range []string{
		"",
		"it-run-tro8w0",
		"it-run-tro8w0-gpgadd",
		"it-run-tro8w0-gk",
		"it-run-tro8w0-gk7100e3d5d90a4d5",
		"it-run-tro8w0-gk7100e3d5d90a4d533",
		"it-run-tro8w0-gk7100e3d5d90a4d5g",
		"it-run-tro8w0-gk7100E3D5D90A4D53",
		"it-run-tro8w0-xgk7100e3d5d90a4d53",
		"x-it-run-tro8w0-gk7100e3d5d90a4d53",
		"it-run--gk7100e3d5d90a4d53",
		"it-run-0-gk7100e3d5d90a4d53",
		"it-run-tro8w0-gk7100e3d5d90a4d53-x",
	} {
		_, _, ok := markerKeyID(name)
		require.False(t, ok, "a name that is not a marker is read as one")
	}
}

func TestRunKeyUserID(t *testing.T) {
	uid, err := runKeyUserID("it-run-tro8w0", "gpgadd")
	require.NoError(t, err)
	require.True(t, uid == "it-run-tro8w0-gpgadd", "the user ID is the run repository's name, a hyphen and the suffix")

	for _, bad := range [][2]string{
		{"it-run-tro8w0", ""},
		{"it-run-tro8w0", "Gpg"},
		{"it-run-tro8w0", "a-b"},
		{"it-run-tro8w0", "a b"},
		{"it-run-tro8w0-x", "y"},
		{"other", "x"},
	} {
		_, err := runKeyUserID(bad[0], bad[1])
		require.Error(t, err)
	}
}

func TestDeleteRunGPGKey_RefusesWithoutMarker(t *testing.T) {
	for _, c := range [][2]string{
		{testKeyID, ""},
		{testKeyID, "it-run-tro8w0"},
		{testKeyID, "it-run-tro8w0-gpgadd"},
		{testKeyID, "it-run-tro8w0-gk0000000000000000"},
		{"0000000000000000", testMarker},
		{"", testMarker},
		{strings.ToLower(testKeyID), testMarker},
		{testKeyID + " ", testMarker},
	} {
		api, calls := recordingAPI(204, "")
		require.Error(t, deleteRunGPGKey(api, 1, c[0], c[1]))
		require.True(t, len(*calls) == 0, "the GPG key delete path sent a request without a marker naming the key")

		api, calls = recordingAPI(200, "[]")
		require.Error(t, deleteRunGPGKeyByKeyID(api, c[0], c[1]))
		require.True(t, len(*calls) == 0, "the GPG key delete by key ID sent a request without a marker naming the key")
	}
}

func TestDeleteRunGPGKey_DeletesMarkedKey(t *testing.T) {
	api, calls := recordingAPI(204, "")
	require.NoError(t, deleteRunGPGKey(api, 42, testKeyID, testMarker))
	require.True(t, slices.Equal(*calls, []string{"DELETE /user/gpg_keys/42"}), "the GPG key delete path sent no single DELETE of the key")

	api, _ = recordingAPI(404, "")
	require.Error(t, deleteRunGPGKey(api, 42, testKeyID, testMarker))
}

func TestDeleteRunGPGKeyByKeyID(t *testing.T) {
	var calls []string
	api := func(method, path string, _ any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		switch method {
		case "DELETE":
			return 204, nil, nil
		case "GET":
			if strings.HasPrefix(path, "/user/gpg_keys?") {
				data, _ := json.Marshal([]map[string]any{{"id": 3, "key_id": "0000000000000001"}, {"id": 5, "key_id": testKeyID}})
				return 200, data, nil
			}
			return 404, nil, nil
		}
		return 500, nil, nil
	}
	require.NoError(t, deleteRunGPGKeyByKeyID(api, testKeyID, testMarker))
	require.True(t, slices.Equal(calls, []string{
		"GET /user/gpg_keys?page=1&limit=50",
		"DELETE /user/gpg_keys/5",
		"GET /user/gpg_keys/5",
	}), "the delete by key ID did not delete only the marked key and read it back")

	// A key that is still there after the delete fails the cleanup.
	still := func(method, path string, _ any) (int, []byte, error) {
		if method == "DELETE" {
			return 204, nil, nil
		}
		data, _ := json.Marshal([]map[string]any{{"id": 5, "key_id": testKeyID}})
		return 200, data, nil
	}
	require.Error(t, deleteRunGPGKeyByKeyID(still, testKeyID, testMarker))

	// No key of that ID: nothing to delete.
	api, recorded := recordingAPI(200, "[]")
	require.NoError(t, deleteRunGPGKeyByKeyID(api, testKeyID, testMarker))
	require.True(t, len(*recorded) == 1, "the delete by key ID sent more than the list request")
}

func TestDeleteStaleRunGPGKeys(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	staleRun := runRepoName(now.Add(-2*time.Hour - time.Second))
	freshRun := runRepoName(now.Add(-2 * time.Hour))
	const (
		staleID   = "00000000000000A1"
		freshID   = "00000000000000B2"
		ownID     = "00000000000000C3"
		unmatched = "00000000000000D4"
	)
	staleMarker, err := gpgKeyMarker(staleRun, staleID)
	require.NoError(t, err)
	freshMarker, err := gpgKeyMarker(freshRun, freshID)
	require.NoError(t, err)
	unmatchedMarker, err := gpgKeyMarker(staleRun, unmatched)
	require.NoError(t, err)

	// Two pages of repositories: the stale marker is on the second.
	var repos []map[string]any
	for i := 0; i < 50; i++ {
		repos = append(repos, map[string]any{"name": fmt.Sprintf("plc-%d", i)})
	}
	repoPage2 := []map[string]any{
		{"name": staleMarker},
		{"name": freshMarker},
		{"name": unmatchedMarker},
		{"name": staleRun},
		{"name": staleRun + "-gpgadd"},
		{"name": strings.ToUpper(staleMarker)},
	}
	// Two pages of keys: the key of the stale marker is on the second.
	var keys []map[string]any
	for i := 0; i < 49; i++ {
		keys = append(keys, map[string]any{"id": 100 + i, "key_id": fmt.Sprintf("%016X", 0x1000+i)})
	}
	keys = append(keys, map[string]any{"id": 9, "key_id": ownID})
	keyPage2 := []map[string]any{
		{"id": 7, "key_id": staleID},
		{"id": 8, "key_id": freshID},
	}
	var calls []string
	api := func(method, path string, _ any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		if method == "DELETE" {
			return 204, nil, nil
		}
		var page any = []map[string]any{}
		switch path {
		case "/orgs/owner/repos?page=1&limit=50":
			page = repos
		case "/orgs/owner/repos?page=2&limit=50":
			page = repoPage2
		case "/user/gpg_keys?page=1&limit=50":
			page = keys
		case "/user/gpg_keys?page=2&limit=50":
			page = keyPage2
		}
		data, _ := json.Marshal(page)
		return 200, data, nil
	}

	require.NoError(t, deleteStaleRunGPGKeys(api, "owner", now))
	require.True(t, slices.Equal(calls, []string{
		"GET /orgs/owner/repos?page=1&limit=50",
		"GET /orgs/owner/repos?page=2&limit=50",
		"GET /user/gpg_keys?page=1&limit=50",
		"GET /user/gpg_keys?page=2&limit=50",
		"DELETE /user/gpg_keys/7",
	}), "the sweep did not read every page and delete only the key of the stale marker")

	// No stale marker: the keys are not even read.
	api, recorded := recordingAPI(200, "[]")
	require.NoError(t, deleteStaleRunGPGKeys(api, "owner", now))
	require.True(t, len(*recorded) == 1, "the sweep read the keys without a stale marker")

	failing, _ := recordingAPI(403, "")
	require.Error(t, deleteStaleRunGPGKeys(failing, "owner", now))
}
