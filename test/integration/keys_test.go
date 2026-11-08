package integration

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// The SSH keys the integration tests add live on the test account, not in
// the run repository, so they outlive a run that hits the timeout. Each key
// is generated in the test, its private half only in memory, and titled
// like a scratch repository of the run, it-run-<base36 time>-<suffix>, so
// the delete path below can tell it from every other key of the account.
// This file carries no build tag, so its unit tests check the delete path
// with every go test run.

// runKeyTime returns the creation time in the title of a run's SSH key, and
// false for any title runKeyTitle does not produce: the title of a run
// repository without a suffix included.
func runKeyTime(title string) (time.Time, bool) {
	created, ok := runRepoTime(title)
	if !ok || !strings.Contains(title[len(runRepoPrefix):], "-") {
		return time.Time{}, false
	}
	return created, true
}

// runKeyTitle is the title of the SSH key with suffix of the run repository
// run. It refuses a title the delete path would refuse: suffix must be 0-9
// and a-z only.
func runKeyTitle(run, suffix string) (string, error) {
	title := run + "-" + suffix
	if _, ok := runRepoTime(run); !ok || strings.Contains(run[len(runRepoPrefix):], "-") {
		return "", errors.New("refusing an SSH key title whose run part is not a run repository's name")
	}
	if _, ok := runKeyTime(title); !ok {
		return "", errors.New("refusing an SSH key title whose suffix is not 0-9 or a-z")
	}
	return title, nil
}

// newPublicKey returns a fresh ed25519 public key in the OpenSSH
// authorized_keys format without a comment. The private half is dropped.
func newPublicKey() (string, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", errors.New("generate an ed25519 key")
	}
	const keyType = "ssh-ed25519"
	var blob []byte
	for _, part := range [][]byte{[]byte(keyType), pub} {
		blob = binary.BigEndian.AppendUint32(blob, uint32(len(part)))
		blob = append(blob, part...)
	}
	return keyType + " " + base64.StdEncoding.EncodeToString(blob), nil
}

// sshKey is the part of an SSH key of GET /user/keys the fixture reads.
type sshKey struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// listSSHKeys reads every page of the SSH keys of the test account. Its
// errors give the status only.
func listSSHKeys(api apiFunc) ([]sshKey, error) {
	const limit = 50
	var all []sshKey
	for page := 1; ; page++ {
		status, data, err := api(http.MethodGet, fmt.Sprintf("/user/keys?page=%d&limit=%d", page, limit), nil)
		if err != nil {
			return nil, fmt.Errorf("list SSH keys: %w", err)
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("list SSH keys: status %d, want %d", status, http.StatusOK)
		}
		var keys []sshKey
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, errors.New("list SSH keys: undecodable response")
		}
		all = append(all, keys...)
		if len(keys) < limit {
			return all, nil
		}
	}
}

// deleteRunKey deletes the SSH key id of the test account, titled title, and
// refuses, without a request, any title that is not a run key's. It is the
// only delete of an SSH key in the integration tests.
func deleteRunKey(api apiFunc, id int64, title string) error {
	if _, ok := runKeyTime(title); !ok {
		return fmt.Errorf("refusing to delete an SSH key whose title does not start with %s, a time and a suffix", runRepoPrefix)
	}
	status, _, err := api(http.MethodDelete, fmt.Sprintf("/user/keys/%d", id), nil)
	if err != nil {
		return fmt.Errorf("delete run SSH key: %w", err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("delete run SSH key: status %d, want %d", status, http.StatusNoContent)
	}
	return nil
}

// deleteStaleRunKeys deletes every SSH key of the test account whose title
// is a run key's created more than runRepoMaxAge before now. It reads all
// pages before the first delete, so no deletion shifts a page it has yet to
// read.
func deleteStaleRunKeys(api apiFunc, now time.Time) error {
	keys, err := listSSHKeys(api)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if created, ok := runKeyTime(k.Title); ok && now.Sub(created) > runRepoMaxAge {
			if err := deleteRunKey(api, k.ID, k.Title); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteRunKeyByTitle deletes the SSH key titled title, a run key's, when
// the account has one, and fails unless the API then answers 404 for it. A
// test's cleanup calls it, since the key was added by the command under
// test and its ID is not known.
func deleteRunKeyByTitle(api apiFunc, title string) error {
	if _, ok := runKeyTime(title); !ok {
		return fmt.Errorf("refusing to delete an SSH key whose title does not start with %s, a time and a suffix", runRepoPrefix)
	}
	keys, err := listSSHKeys(api)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k.Title != title {
			continue
		}
		if err := deleteRunKey(api, k.ID, k.Title); err != nil {
			return err
		}
		status, _, err := api(http.MethodGet, fmt.Sprintf("/user/keys/%d", k.ID), nil)
		if err != nil {
			return fmt.Errorf("read deleted run SSH key: %w", err)
		}
		if status != http.StatusNotFound {
			return fmt.Errorf("read deleted run SSH key: status %d, want %d", status, http.StatusNotFound)
		}
	}
	return nil
}
