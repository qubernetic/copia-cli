//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	issueclose "github.com/qubernetic/copia-cli/pkg/cmd/issue/close"
	issuecomment "github.com/qubernetic/copia-cli/pkg/cmd/issue/comment"
	issuecreate "github.com/qubernetic/copia-cli/pkg/cmd/issue/create"
	issueedit "github.com/qubernetic/copia-cli/pkg/cmd/issue/edit"
	issuelist "github.com/qubernetic/copia-cli/pkg/cmd/issue/list"
	issuereopen "github.com/qubernetic/copia-cli/pkg/cmd/issue/reopen"
	issueview "github.com/qubernetic/copia-cli/pkg/cmd/issue/view"
)

func TestIssue_FullLifecycle(t *testing.T) {
	env := loadTestEnv(t)

	// Create
	createIO, createOut, _ := testIO()
	createOpts := &issuecreate.CreateOptions{
		IO:         createIO,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		Title:      "[integration-test] CLI lifecycle test",
		Body:       "Created by integration test via CreateRun.",
	}
	err := issuecreate.CreateRun(createOpts)
	require.NoError(t, err)
	assert.Contains(t, createOut.String(), "Created issue #")
	t.Logf("Create: %s", createOut.String())

	// Extract issue number from output
	var issueNumber int64
	_, _ = fmt.Sscanf(createOut.String(), "Created issue #%d", &issueNumber)
	require.Greater(t, issueNumber, int64(0), "failed to parse issue number")

	// Cleanup: close at end
	defer func() {
		closeIO, _, _ := testIO()
		_ = issueclose.CloseRun(&issueclose.CloseOptions{
			IO: closeIO, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: issueNumber,
		})
		t.Logf("Closed issue #%d", issueNumber)
	}()

	// View
	viewIO, viewOut, _ := testIO()
	err = issueview.ViewRun(&issueview.ViewOptions{
		IO: viewIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
	})
	require.NoError(t, err)
	assert.Contains(t, viewOut.String(), "[integration-test] CLI lifecycle test")
	t.Logf("View: %s", viewOut.String())

	// Comment
	commentIO, _, _ := testIO()
	err = issuecomment.CommentRun(&issuecomment.CommentOptions{
		IO: commentIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
		Body:   "Integration test comment via CommentRun.",
	})
	require.NoError(t, err)
	t.Logf("Comment added to #%d", issueNumber)

	// Edit title
	editIO, _, _ := testIO()
	err = issueedit.EditRun(&issueedit.EditOptions{
		IO: editIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
		Title:  "[integration-test] CLI lifecycle test (edited)",
	})
	require.NoError(t, err)
	t.Logf("Edited issue #%d", issueNumber)

	// Close
	closeIO, closeOut, _ := testIO()
	err = issueclose.CloseRun(&issueclose.CloseOptions{
		IO: closeIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
	})
	require.NoError(t, err)
	assert.Contains(t, closeOut.String(), "Closed issue")
	t.Logf("Close: %s", closeOut.String())

	// Reopen with a comment; the deferred cleanup closes it again
	reopenIO, reopenOut, _ := testIO()
	err = issuereopen.ReopenRun(&issuereopen.ReopenOptions{
		IO: reopenIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number:  issueNumber,
		Comment: "Reopened by integration test via ReopenRun.",
	})
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("Reopened issue #%d\n", issueNumber), reopenOut.String())
	t.Logf("Reopen: %s", reopenOut.String())

	// Reopen again: the issue is open, so nothing is sent
	againIO, againOut, againErr := testIO()
	err = issuereopen.ReopenRun(&issuereopen.ReopenOptions{
		IO: againIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
	})
	require.NoError(t, err)
	assert.Empty(t, againOut.String())
	assert.Contains(t, againErr.String(), "is already open")
}

func TestIssue_List(t *testing.T) {
	env := loadTestEnv(t)
	ios, stdout, _ := testIO()

	opts := &issuelist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		State:      "all",
		Limit:      5,
	}

	err := issuelist.ListRun(opts)
	require.NoError(t, err)
	t.Logf("Output: %s", stdout.String())
}

func TestIssue_ViewNotFound(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	err := issueview.ViewRun(&issueview.ViewOptions{
		IO: ios, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: 99999,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
