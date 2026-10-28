package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ResolveAssignees replaces every `@me` in logins with the login of the
// authenticated user, read with one GET /user. Without `@me` it sends no
// request.
func ResolveAssignees(client *http.Client, host, token string, logins []string) ([]string, error) {
	var me string
	out := make([]string, 0, len(logins))
	for _, login := range logins {
		if login == "@me" {
			if me == "" {
				var err error
				if me, err = currentLogin(client, host, token); err != nil {
					return nil, err
				}
			}
			login = me
		}
		out = append(out, login)
	}
	return out, nil
}

func currentLogin(client *http.Client, host, token string) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/api/v1/user", host), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to resolve @me (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return "", err
	}
	return user.Login, nil
}
