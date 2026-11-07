package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
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
	if tag == "" {
		return fetchLatestRelease(client, host, token, owner, repo)
	}
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/tags/%s", host, owner, repo, url.PathEscape(tag))

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

// fetchLatestRelease returns the newest release of owner/repo that is
// neither a draft nor a pre-release, the release GitHub calls the latest.
// Copia answers GET /releases/latest with 404 even then, so it comes from
// the release list, which is newest first.
func fetchLatestRelease(client *http.Client, host, token, owner, repo string) (*Release, error) {
	u := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases?draft=false&pre-release=false", host, owner, repo)
	decode := func(body []byte) ([]Release, error) {
		var releases []Release
		err := json.Unmarshal(body, &releases)
		return releases, err
	}
	published := func(r Release) bool { return !r.Draft && !r.Prerelease }

	releases, err := cmdutil.FetchFilteredPages(client, u, token, 1, decode, published)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, errors.New("release not found")
	}
	return &releases[0], nil
}
