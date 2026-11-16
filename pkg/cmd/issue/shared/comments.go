package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Comment is one conversation comment of an issue or pull request.
type Comment struct {
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	CreatedAt string `json:"created_at"`
}

// FetchComments returns every comment of issue or pull request number, oldest
// first. The endpoint takes no paging, so one request returns all of them; a
// pull request shares the issue index, so this serves its comments too.
func FetchComments(client *http.Client, host, token, owner, repo string, number int64) ([]Comment, error) {
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/comments", host, owner, repo, number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var comments []Comment
	if err := json.Unmarshal(body, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// PrintComments writes each comment as a blank line, a
// `<login> commented <created_at>:` line and the comment body.
func PrintComments(w io.Writer, comments []Comment) {
	for _, c := range comments {
		_, _ = fmt.Fprintf(w, "\n%s commented %s:\n%s\n", c.User.Login, c.CreatedAt, c.Body)
	}
}
