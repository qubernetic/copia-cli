//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	releasecreate "github.com/qubernetic/copia-cli/pkg/cmd/release/create"
	releasedelete "github.com/qubernetic/copia-cli/pkg/cmd/release/delete"
	releasedeleteasset "github.com/qubernetic/copia-cli/pkg/cmd/release/deleteasset"
	releaseedit "github.com/qubernetic/copia-cli/pkg/cmd/release/edit"
	releaselist "github.com/qubernetic/copia-cli/pkg/cmd/release/list"
	releaseupload "github.com/qubernetic/copia-cli/pkg/cmd/release/upload"
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

	// Edit title and draft state, view shows them, then publish again
	editedTitle := "Integration Test Release (edited)"
	for _, draft := range []bool{true, false} {
		editIO, editOut, _ := testIO()
		err = releaseedit.EditRun(&releaseedit.EditOptions{
			IO:         editIO,
			HTTPClient: &http.Client{},
			Host:       env.Host,
			Token:      env.Token,
			Owner:      env.Owner,
			Repo:       env.Repo,
			Tag:        tag,
			Title:      &editedTitle,
			Draft:      &draft,
		})
		require.NoError(t, err, "release edit")
		assertOutputContains(t, editOut.String(), "/releases/tag/"+tag+"\n")

		viewIO, viewOut, _ := testIO()
		err = releaseview.ViewRun(&releaseview.ViewOptions{
			IO:         viewIO,
			HTTPClient: &http.Client{},
			Host:       env.Host,
			Token:      env.Token,
			Owner:      env.Owner,
			Repo:       env.Repo,
			Tag:        tag,
		})
		require.NoError(t, err, "release view after edit")
		assertOutputContains(t, viewOut.String(), "title:\t"+editedTitle+"\n")
		assertOutputContains(t, viewOut.String(), fmt.Sprintf("draft:\t%t\n", draft))
	}

	// Upload an asset, delete it with delete-asset, view shows no asset
	assetName := "integration-asset.txt"
	assetPath := filepath.Join(t.TempDir(), assetName)
	require.NoError(t, os.WriteFile(assetPath, []byte("integration test asset\n"), 0o600), "write asset file")

	uploadIO, uploadOut, _ := testIO()
	err = releaseupload.UploadRun(&releaseupload.UploadOptions{
		IO:         uploadIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Tag:        tag,
		Files:      []string{assetPath},
	})
	require.NoError(t, err, "release upload")
	assertOutputContains(t, uploadOut.String(), "Uploaded "+assetName)

	assetViewIO, assetViewOut, _ := testIO()
	err = releaseview.ViewRun(&releaseview.ViewOptions{
		IO:         assetViewIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Tag:        tag,
	})
	require.NoError(t, err, "release view after upload")
	assertOutputContains(t, assetViewOut.String(), "asset:\t"+assetName+"\n")

	deleteAssetIO, deleteAssetOut, _ := testIO()
	err = releasedeleteasset.DeleteAssetRun(&releasedeleteasset.DeleteAssetOptions{
		IO:         deleteAssetIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Tag:        tag,
		AssetName:  assetName,
		Confirmed:  true,
	})
	require.NoError(t, err, "release delete-asset")
	assertOutputEqual(t, deleteAssetOut.String(), "Deleted asset "+assetName+" from release "+tag+"\n")

	noAssetViewIO, noAssetViewOut, _ := testIO()
	err = releaseview.ViewRun(&releaseview.ViewOptions{
		IO:         noAssetViewIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Tag:        tag,
	})
	require.NoError(t, err, "release view after delete-asset")
	assertOutputNotContains(t, noAssetViewOut.String(), "asset:\t")

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
