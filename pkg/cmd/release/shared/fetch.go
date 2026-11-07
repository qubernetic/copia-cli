package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Author is the user who created a release.
type Author struct {
	Login string `json:"login"`
}

// Asset is a file attached to a release.
type Asset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadCount      int64  `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is a repository release with the fields of `release view --json`.
type Release struct {
	ID              int64   `json:"id"`
	TagName         string  `json:"tag_name"`
	Name            string  `json:"name"`
	Body            string  `json:"body"`
	Draft           bool    `json:"draft"`
	Prerelease      bool    `json:"prerelease"`
	CreatedAt       string  `json:"created_at"`
	PublishedAt     string  `json:"published_at"`
	Author          Author  `json:"author"`
	HTMLURL         string  `json:"html_url"`
	TarballURL      string  `json:"tarball_url"`
	ZipballURL      string  `json:"zipball_url"`
	TargetCommitish string  `json:"target_commitish"`
	Assets          []Asset `json:"assets"`
}

// ReleaseFields are the field names `--json` accepts for a release.
var ReleaseFields = []string{
	"id", "tag_name", "name", "body", "draft", "prerelease", "created_at", "published_at",
	"author", "html_url", "tarball_url", "zipball_url", "target_commitish", "assets",
}

// FetchRelease returns the release of owner/repo with tag, or the latest
// release when tag is empty.
func FetchRelease(client *http.Client, host, token, owner, repo, tag string) (*Release, error) {
	path := "latest"
	if tag != "" {
		path = "tags/" + url.PathEscape(tag)
	}
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/%s", host, owner, repo, path)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		if tag == "" {
			return nil, fmt.Errorf("release not found")
		}
		return nil, fmt.Errorf("release %s not found", tag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, err
	}
	return &release, nil
}
