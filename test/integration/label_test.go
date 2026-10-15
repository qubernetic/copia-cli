//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	labelcreate "github.com/qubernetic/copia-cli/pkg/cmd/label/create"
	labellist "github.com/qubernetic/copia-cli/pkg/cmd/label/list"
)

func TestLabel_CreateAndList(t *testing.T) {
	env := loadTestEnv(t)

	// Create label via CLI
	createIO, createOut, _ := testIO()
	err := labelcreate.CreateRun(&labelcreate.CreateOptions{
		IO: createIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Name: "integration-test-label", Color: "#ff0000",
	})
	require.NoError(t, err)
	assert.Contains(t, createOut.String(), "integration-test-label")
	t.Logf("Create: %s", createOut.String())

	// Cleanup: delete label via API
	defer func() {
		// Get label ID first
		listURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/labels", env.Host, env.Owner, env.Repo)
		req, _ := http.NewRequest("GET", listURL, nil)
		req.Header.Set("Authorization", "token "+env.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()
		var labels []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&labels)
		for _, l := range labels {
			if l.Name == "integration-test-label" {
				delURL := fmt.Sprintf("%s/%d", listURL, l.ID)
				delReq, _ := http.NewRequest("DELETE", delURL, nil)
				delReq.Header.Set("Authorization", "token "+env.Token)
				delResp, _ := http.DefaultClient.Do(delReq)
				if delResp != nil {
					_ = delResp.Body.Close()
				}
				t.Logf("Deleted label ID: %d", l.ID)
			}
		}
	}()

	// List labels via CLI
	listIO, listOut, _ := testIO()
	err = labellist.ListRun(&labellist.ListOptions{
		IO: listIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
	})
	require.NoError(t, err)
	assert.Contains(t, listOut.String(), "integration-test-label")
	t.Logf("List: %s", listOut.String())
}
