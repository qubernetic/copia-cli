// Package api provides a Gitea SDK client wrapper for the Copia REST API.
package api

import (
	"fmt"
	"net/http"

	"code.gitea.io/sdk/gitea"
)

// NewClientWithHTTP creates a Gitea SDK client that sends its requests
// through httpClient. Pass cmdutil.NewHTTPClient() outside of tests, so
// redirects stay on the host of the token.
func NewClientWithHTTP(host, token string, httpClient *http.Client) (*gitea.Client, error) {
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	url := "https://" + host
	return gitea.NewClient(url, gitea.SetToken(token), gitea.SetHTTPClient(httpClient))
}
