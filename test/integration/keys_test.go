package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// The GPG keys the integration tests add live on the test account, not in
// the run repository, so they outlive a run that hits the timeout. Gitea
// returns no user ID of a key (its public_key holds the primary key packet
// only), so nothing the API returns tells a test's key from the owner's.
// Before a test adds a key, it creates a marker: a private scratch
// repository it-run-<base36 time>-gk<key ID in lower case>. Gitea refuses a
// key ID that any account already uses, so a key of the test account whose
// ID a marker names is the key the test generated. Every delete of a GPG key
// needs the marker that names it. This file carries no build tag, so its
// unit tests check the delete path with every go test run.

// markerPrefix starts the suffix of a GPG key marker; the key ID follows.
const markerPrefix = "gk"

// keyIDLen is the length of a GPG key ID in hex digits, as Gitea returns it.
const keyIDLen = 16

// isKeyID reports whether s is a GPG key ID as Gitea returns it: 16 hex
// digits in upper case.
func isKeyID(s string) bool {
	if len(s) != keyIDLen {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// isRunName reports whether run is the name of a run repository, without
// the suffix of a scratch repository.
func isRunName(run string) bool {
	_, ok := runRepoTime(run)
	return ok && !strings.Contains(run[len(runRepoPrefix):], "-")
}

// gpgKeyMarker is the name of the marker of the key keyID in the run
// repository run. It refuses a name markerKeyID would not read back.
func gpgKeyMarker(run, keyID string) (string, error) {
	if !isRunName(run) || !isKeyID(keyID) {
		return "", errors.New("refusing a GPG key marker that is not a run repository's name, -gk and a key ID")
	}
	return run + "-" + markerPrefix + strings.ToLower(keyID), nil
}

// markerKeyID returns the key ID, in upper case, and the creation time in
// the name of a GPG key marker, and false for any name gpgKeyMarker does not
// produce.
func markerKeyID(name string) (string, time.Time, bool) {
	created, ok := runRepoTime(name)
	if !ok {
		return "", time.Time{}, false
	}
	_, suffix, _ := strings.Cut(name[len(runRepoPrefix):], "-")
	hex, ok := strings.CutPrefix(suffix, markerPrefix)
	if !ok || hex != strings.ToLower(hex) || !isKeyID(strings.ToUpper(hex)) {
		return "", time.Time{}, false
	}
	return strings.ToUpper(hex), created, true
}

// runKeyUserID is the user ID of the GPG key with suffix of the run
// repository run. Gitea does not return it; it only shows a key's origin to
// whoever reads the key itself.
func runKeyUserID(run, suffix string) (string, error) {
	if !isRunName(run) || !isBase36Digits(suffix) {
		return "", errors.New("refusing a GPG key user ID that is not a run repository's name, a hyphen and 0-9 or a-z")
	}
	return run + "-" + suffix, nil
}

// gpgKey is the part of a GPG key of GET /user/gpg_keys the fixture reads.
type gpgKey struct {
	ID    int64  `json:"id"`
	KeyID string `json:"key_id"`
}

// listGPGKeys reads every page of the GPG keys of the test account. Its
// errors give the status only.
func listGPGKeys(api apiFunc) ([]gpgKey, error) {
	const limit = 50
	var all []gpgKey
	for page := 1; ; page++ {
		status, data, err := api(http.MethodGet, fmt.Sprintf("/user/gpg_keys?page=%d&limit=%d", page, limit), nil)
		if err != nil {
			return nil, fmt.Errorf("list GPG keys: %w", err)
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("list GPG keys: status %d, want %d", status, http.StatusOK)
		}
		var keys []gpgKey
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, errors.New("list GPG keys: undecodable response")
		}
		all = append(all, keys...)
		if len(keys) < limit {
			return all, nil
		}
	}
}

// deleteRunGPGKey deletes the GPG key id of the test account, whose key ID
// is keyID, and refuses, without a request, unless marker is a GPG key
// marker naming keyID. It is the only delete of a GPG key in the
// integration tests.
func deleteRunGPGKey(api apiFunc, id int64, keyID, marker string) error {
	if named, _, ok := markerKeyID(marker); !ok || named != keyID {
		return errors.New("refusing to delete a GPG key that no marker of a run names")
	}
	status, _, err := api(http.MethodDelete, fmt.Sprintf("/user/gpg_keys/%d", id), nil)
	if err != nil {
		return fmt.Errorf("delete run GPG key: %w", err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("delete run GPG key: status %d, want %d", status, http.StatusNoContent)
	}
	return nil
}

// deleteStaleRunGPGKeys deletes every GPG key of the test account that a
// marker of the organization owner names, created more than runRepoMaxAge
// before now. It runs before deleteStaleRunRepos, which deletes the markers
// then. It reads all pages before the first delete, so no deletion shifts a
// page it has yet to read.
func deleteStaleRunGPGKeys(api apiFunc, owner string, now time.Time) error {
	names, err := listOrgRepos(api, owner)
	if err != nil {
		return err
	}
	markers := map[string]string{}
	for _, name := range names {
		if keyID, created, ok := markerKeyID(name); ok && now.Sub(created) > runRepoMaxAge {
			markers[keyID] = name
		}
	}
	if len(markers) == 0 {
		return nil
	}
	keys, err := listGPGKeys(api)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if marker, ok := markers[k.KeyID]; ok {
			if err := deleteRunGPGKey(api, k.ID, k.KeyID, marker); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteRunGPGKeyByKeyID deletes the GPG key keyID, which marker names, when
// the account has it, and fails unless the API then answers 404 for it. A
// test's cleanup calls it, since the key was added by the command under
// test and its ID in the API is not known.
func deleteRunGPGKeyByKeyID(api apiFunc, keyID, marker string) error {
	if named, _, ok := markerKeyID(marker); !ok || named != keyID {
		return errors.New("refusing to delete a GPG key that no marker of a run names")
	}
	keys, err := listGPGKeys(api)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k.KeyID != keyID {
			continue
		}
		if err := deleteRunGPGKey(api, k.ID, k.KeyID, marker); err != nil {
			return err
		}
		status, _, err := api(http.MethodGet, fmt.Sprintf("/user/gpg_keys/%d", k.ID), nil)
		if err != nil {
			return fmt.Errorf("read deleted run GPG key: %w", err)
		}
		if status != http.StatusNotFound {
			return fmt.Errorf("read deleted run GPG key: status %d, want %d", status, http.StatusNotFound)
		}
	}
	return nil
}
