//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	labelcreate "github.com/qubernetic/copia-cli/pkg/cmd/label/create"
	labeldelete "github.com/qubernetic/copia-cli/pkg/cmd/label/delete"
	labeledit "github.com/qubernetic/copia-cli/pkg/cmd/label/edit"
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
	assertOutputContains(t, createOut.String(), "integration-test-label")

	t.Cleanup(func() { deleteLabelAtCleanup(t, env, "integration-test-label") })

	// List labels via CLI
	listIO, listOut, _ := testIO()
	err = labellist.ListRun(&labellist.ListOptions{
		IO: listIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
	})
	require.NoError(t, err)
	assertOutputContains(t, listOut.String(), "integration-test-label")
}

func TestLabel_Edit(t *testing.T) {
	env := loadTestEnv(t)

	createIO, _, _ := testIO()
	err := labelcreate.CreateRun(&labelcreate.CreateOptions{
		IO: createIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Name: "integration-edit-label", Color: "#ff0000",
	})
	require.NoError(t, err, "create the label to edit")

	editIO, editOut, _ := testIO()
	err = labeledit.EditRun(&labeledit.EditOptions{
		IO: editIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Name: "integration-edit-label", NewName: "integration-edited-label",
		Color: "#00aa55", Description: "Edited by the integration test",
	})
	require.NoError(t, err, "edit the label")
	t.Cleanup(func() { deleteLabelAtCleanup(t, env, "integration-edited-label") })
	assertOutputContains(t, editOut.String(), `Label "integration-edit-label" updated in `)

	listIO, listOut, _ := testIO()
	err = labellist.ListRun(&labellist.ListOptions{
		IO: listIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
	})
	require.NoError(t, err, "list the labels")
	assertOutputContains(t, listOut.String(), "integration-edited-label")
	assertOutputContains(t, listOut.String(), "00aa55")
	assertOutputContains(t, listOut.String(), "Edited by the integration test")
}

// deleteLabelAtCleanup removes a label the test created with label delete.
func deleteLabelAtCleanup(t *testing.T, env testEnv, name string) {
	t.Helper()
	ios, _, _ := testIO()
	err := labeldelete.DeleteRun(&labeldelete.DeleteOptions{
		IO: ios, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Name: name, Confirmed: true,
	})
	assert.NoError(t, err, "delete the test label at cleanup")
}

func TestLabel_Delete(t *testing.T) {
	env := loadTestEnv(t)

	createIO, _, _ := testIO()
	err := labelcreate.CreateRun(&labelcreate.CreateOptions{
		IO: createIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Name: "integration-delete-label", Color: "#123456",
	})
	require.NoError(t, err, "create the label to delete")

	deleteIO, deleteOut, _ := testIO()
	err = labeldelete.DeleteRun(&labeldelete.DeleteOptions{
		IO: deleteIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Name: "integration-delete-label", Confirmed: true,
	})
	require.NoError(t, err, "delete the label")
	assertOutputContains(t, deleteOut.String(), `Label "integration-delete-label" deleted from `)

	listIO, listOut, _ := testIO()
	err = labellist.ListRun(&labellist.ListOptions{
		IO: listIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Limit: 100,
	})
	require.NoError(t, err, "list the labels")
	assert.False(t, strings.Contains(listOut.String(), "integration-delete-label"), "label list still shows the deleted label")
}
