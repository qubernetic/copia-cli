package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ResolveMilestoneID maps a milestone name or numeric ID to the milestone ID.
// Gitea's GET /repos/{owner}/{repo}/milestones/{id} answers for both a name
// and an ID, so one request covers either form.
func ResolveMilestoneID(client *http.Client, host, token, owner, repo, milestone string) (int64, error) {
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/milestones/%s", host, owner, repo, url.PathEscape(milestone))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("connecting to %s: %w", host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("milestone %q not found in %s/%s", milestone, owner, repo)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to look up milestone %q (HTTP %d)", milestone, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var m struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return 0, err
	}
	return m.ID, nil
}
