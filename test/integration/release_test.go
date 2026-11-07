//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	releasecreate "github.com/qubernetic/copia-cli/pkg/cmd/release/create"
	releasedelete "github.com/qubernetic/copia-cli/pkg/cmd/release/delete"
	releaselist "github.com/qubernetic/copia-cli/pkg/cmd/release/list"
	releaseview "github.com/qubernetic/copia-cli/pkg/cmd/release/view"
)

func TestRelease_Lifecycle(t *testing.T) {
	env := loadTestEnv(t)
	tag := "v0.0.0-integration-test"

	// Create release
	createIO, createOut, _ := testIO()
	err := releasecreate.CreateRun(&releasecreate.CreateOptions{
		IO:         createIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Tag:        tag,
		Title:      "Integration Test Release",
		Notes:      "Created by integration test. Will be deleted.",
	})
	require.NoError(t, err)
	assertOutputContains(t, createOut.String(), tag)

	// Cleanup: delete at end
	defer func() {
		deleteIO, _, _ := testIO()
		_ = releasedelete.DeleteRun(&releasedelete.DeleteOptions{
			IO:         deleteIO,
			HTTPClient: &http.Client{},
			Host:       env.Host,
			Token:      env.Token,
			Owner:      env.Owner,
			Repo:       env.Repo,
			Tag:        tag,
		})
	}()

	// List releases
	listIO, listOut, _ := testIO()
	err = releaselist.ListRun(&releaselist.ListOptions{
		IO:         listIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Limit:      10,
	})
	require.NoError(t, err)
	assertOutputContains(t, listOut.String(), tag)

	// View release by tag, then as the latest (the only release of the run repository)
	for _, viewTag := range []string{tag, ""} {
		viewIO, viewOut, _ := testIO()
		err = releaseview.ViewRun(&releaseview.ViewOptions{
			IO:         viewIO,
			HTTPClient: &http.Client{},
			Host:       env.Host,
			Token:      env.Token,
			Owner:      env.Owner,
			Repo:       env.Repo,
			Tag:        viewTag,
		})
		require.NoError(t, err, "release view")
		assertOutputContains(t, viewOut.String(), "tag:\t"+tag+"\n")
		assertOutputContains(t, viewOut.String(), "title:\tIntegration Test Release\n")
	}

	// Delete release
	deleteIO, deleteOut, _ := testIO()
	err = releasedelete.DeleteRun(&releasedelete.DeleteOptions{
		IO:         deleteIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Tag:        tag,
	})
	require.NoError(t, err)
	assertOutputContains(t, deleteOut.String(), "Deleted")
}
