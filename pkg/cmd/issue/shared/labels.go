// Package shared holds helpers used by more than one issue subcommand.
package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

type labelEntry struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func decodeLabels(body []byte) ([]labelEntry, error) {
	var labels []labelEntry
	err := json.Unmarshal(body, &labels)
	return labels, err
}

// ResolveLabelIDs maps label names to IDs using the repository labels and the
// labels of the owning organization. A repository label wins over an
// organization label of the same name. When owner is a user, the organization
// endpoint answers 404 and only repository labels are used.
func ResolveLabelIDs(client *http.Client, host, token, owner, repo string, names []string) ([]int64, error) {
	// A limit of 0 reads every page, so names past the first page resolve.
	repoURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels", host, owner, repo)
	repoLabels, err := cmdutil.FetchPages(client, repoURL, token, 0, decodeLabels)
	if err != nil {
		return nil, err
	}

	orgURL := fmt.Sprintf("https://%s/api/v1/orgs/%s/labels", host, owner)
	orgLabels, err := cmdutil.FetchPages(client, orgURL, token, 0, decodeLabels)
	var httpErr *cmdutil.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		orgLabels, err = nil, nil
	}
	if err != nil {
		return nil, err
	}

	nameToID := map[string]int64{}
	for _, l := range orgLabels {
		nameToID[l.Name] = l.ID
	}
	for _, l := range repoLabels {
		nameToID[l.Name] = l.ID
	}

	var ids []int64
	for _, name := range names {
		id, ok := nameToID[name]
		if !ok {
			return nil, fmt.Errorf("label %q not found in repository or organization", name)
		}
		ids = append(ids, id)
	}

	return ids, nil
}
