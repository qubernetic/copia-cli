package integration

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests carry no build tag: they run with the unit tests and need no
// Copia account, so a change that lets the key delete path reach another
// key of the test account fails before any integration run.

func TestDeleteRunKey_RefusesOtherTitles(t *testing.T) {
	for _, title := range []string{
		"",
		"laptop",
		"it-run-",
		"it-run-tro8w0",
		"it-run-tro8w0-",
		"it-run-tro8w0-X",
		"it-run-tro8w0-a b",
		"it-run-tro8w0-a-b",
		"IT-RUN-tro8w0-x",
		"x-it-run-tro8w0-x",
		"it-run--x",
		"it-run-0-x",
		"it-run-tro8w0-x\n",
	} {
		api, calls := recordingAPI(204, "")
		require.Error(t, deleteRunKey(api, 1, title))
		require.True(t, len(*calls) == 0, "the key delete path sent a request for a title it must refuse")

		api, calls = recordingAPI(200, "[]")
		require.Error(t, deleteRunKeyByTitle(api, title))
		require.True(t, len(*calls) == 0, "the key delete by title sent a request for a title it must refuse")
	}
}

func TestDeleteRunKey_DeletesRunKey(t *testing.T) {
	api, calls := recordingAPI(204, "")
	require.NoError(t, deleteRunKey(api, 42, "it-run-tro8w0-sshadd"))
	require.True(t, slices.Equal(*calls, []string{"DELETE /user/keys/42"}), "the key delete path sent no single DELETE of the key")

	api, _ = recordingAPI(404, "")
	require.Error(t, deleteRunKey(api, 42, "it-run-tro8w0-sshadd"))
}

func TestRunKeyTitle(t *testing.T) {
	title, err := runKeyTitle("it-run-tro8w0", "sshadd")
	require.NoError(t, err)
	require.True(t, title == "it-run-tro8w0-sshadd", "the key title is the run repository's name, a hyphen and the suffix")

	for _, bad := range [][2]string{
		{"it-run-tro8w0", ""},
		{"it-run-tro8w0", "Ssh"},
		{"it-run-tro8w0", "a-b"},
		{"it-run-tro8w0-x", "y"},
		{"other", "x"},
		{"", "x"},
	} {
		_, err := runKeyTitle(bad[0], bad[1])
		require.Error(t, err)
	}
}

func TestNewPublicKey(t *testing.T) {
	a, err := newPublicKey()
	require.NoError(t, err)
	b, err := newPublicKey()
	require.NoError(t, err)
	require.False(t, a == b, "two generated keys are equal")

	fields := strings.Fields(a)
	require.True(t, len(fields) == 2 && fields[0] == "ssh-ed25519", "the key is not an ssh-ed25519 line without a comment")
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	require.NoError(t, err)
	// uint32 length, "ssh-ed25519", uint32 length, the 32-byte public key.
	require.True(t, len(blob) == 4+11+4+ed25519.PublicKeySize, "the key blob has the wrong length")
	require.True(t, string(blob[4:15]) == "ssh-ed25519", "the key blob does not name its type")
}

func TestDeleteStaleRunKeys(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	stale := runRepoName(now.Add(-2*time.Hour-time.Second)) + "-sshadd"
	fresh := runRepoName(now.Add(-2*time.Hour)) + "-sshadd"
	// Two pages of 50: the first holds the stale run key, the second the
	// rest, so the sweep has to read past the first page.
	var first []map[string]any
	for i := 0; i < 49; i++ {
		first = append(first, map[string]any{"id": 100 + i, "title": fmt.Sprintf("laptop-%d", i)})
	}
	first = append(first, map[string]any{"id": 7, "title": stale})
	second := []map[string]any{
		{"id": 8, "title": fresh},
		{"id": 9, "title": runRepoName(now.Add(-3 * time.Hour))},
		{"id": 10, "title": "x" + stale},
		{"id": 11, "title": strings.ToUpper(stale)},
	}
	var calls []string
	api := func(method, path string, _ any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		if method == "DELETE" {
			return 204, nil, nil
		}
		var page any = []map[string]any{}
		switch path {
		case "/user/keys?page=1&limit=50":
			page = first
		case "/user/keys?page=2&limit=50":
			page = second
		}
		data, _ := json.Marshal(page)
		return 200, data, nil
	}

	require.NoError(t, deleteStaleRunKeys(api, now))
	require.True(t, slices.Equal(calls, []string{
		"GET /user/keys?page=1&limit=50",
		"GET /user/keys?page=2&limit=50",
		"DELETE /user/keys/7",
	}), "the sweep did not read both pages and delete only the stale run key")

	failing, _ := recordingAPI(403, "")
	require.Error(t, deleteStaleRunKeys(failing, now))
}

func TestDeleteRunKeyByTitle(t *testing.T) {
	const title = "it-run-tro8w0-sshadd"
	var calls []string
	api := func(method, path string, _ any) (int, []byte, error) {
		calls = append(calls, method+" "+path)
		switch method {
		case "DELETE":
			return 204, nil, nil
		case "GET":
			if strings.HasPrefix(path, "/user/keys?") {
				data, _ := json.Marshal([]map[string]any{{"id": 3, "title": "laptop"}, {"id": 5, "title": title}})
				return 200, data, nil
			}
			return 404, nil, nil
		}
		return 500, nil, nil
	}
	require.NoError(t, deleteRunKeyByTitle(api, title))
	require.True(t, slices.Equal(calls, []string{
		"GET /user/keys?page=1&limit=50",
		"DELETE /user/keys/5",
		"GET /user/keys/5",
	}), "the delete by title did not delete only the key of that title and read it back")

	// A key that is still there after the delete fails the cleanup.
	still := func(method, path string, _ any) (int, []byte, error) {
		if method == "DELETE" {
			return 204, nil, nil
		}
		data, _ := json.Marshal([]map[string]any{{"id": 5, "title": title}})
		return 200, data, nil
	}
	require.Error(t, deleteRunKeyByTitle(still, title))

	// No key of that title: nothing to delete.
	api, recorded := recordingAPI(200, "[]")
	require.NoError(t, deleteRunKeyByTitle(api, title))
	require.True(t, len(*recorded) == 1, "the delete by title sent more than the list request")
}
