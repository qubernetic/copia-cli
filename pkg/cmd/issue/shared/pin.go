package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PinState is what `issue pin` and `issue unpin` read of an issue before
// they change it.
type PinState struct {
	Title    string `json:"title"`
	PinOrder *int   `json:"pin_order"`
}

// Pinned reports whether the issue is pinned. known is false when the server
// does not send `pin_order`, so the state cannot be told from the issue.
func (s PinState) Pinned() (pinned, known bool) {
	if s.PinOrder == nil {
		return false, false
	}
	return *s.PinOrder > 0, true
}

// FetchPinState reads the issue with GET /repos/{owner}/{repo}/issues/{index}.
func FetchPinState(client *http.Client, host, token, owner, repo string, number int64) (*PinState, error) {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d", host, owner, repo, number)
	resp, err := pinRequest(client, "GET", url, token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	issue := fmt.Sprintf("%s/%s#%d", owner, repo, number)
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("issue %s not found", issue)
	default:
		return nil, fmt.Errorf("failed to read issue %s (HTTP %d)", issue, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var state PinState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SetPinned pins the issue with POST /repos/{owner}/{repo}/issues/{index}/pin,
// or unpins it with DELETE on the same path.
func SetPinned(client *http.Client, host, token, owner, repo string, number int64, pin bool) error {
	method, verb := "DELETE", "unpin"
	if pin {
		method, verb = "POST", "pin"
	}
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/pin", host, owner, repo, number)
	resp, err := pinRequest(client, method, url, token)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	issue := fmt.Sprintf("%s/%s#%d", owner, repo, number)
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("cannot %s issue %s: permission denied (HTTP 403); %sning an issue needs write access to the repository's issues", verb, issue, verb)
	case http.StatusNotFound:
		return fmt.Errorf("issue %s not found", issue)
	}

	var apiErr struct {
		Message string `json:"message"`
	}
	if body, err := io.ReadAll(resp.Body); err == nil && json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("failed to %s issue %s (HTTP %d): %s", verb, issue, resp.StatusCode, apiErr.Message)
	}
	return fmt.Errorf("failed to %s issue %s (HTTP %d)", verb, issue, resp.StatusCode)
}

func pinRequest(client *http.Client, method, url, token string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	return client.Do(req)
}
