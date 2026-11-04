//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	issueclose "github.com/qubernetic/copia-cli/pkg/cmd/issue/close"
	issuecomment "github.com/qubernetic/copia-cli/pkg/cmd/issue/comment"
	issuecreate "github.com/qubernetic/copia-cli/pkg/cmd/issue/create"
	issuedelete "github.com/qubernetic/copia-cli/pkg/cmd/issue/delete"
	issueedit "github.com/qubernetic/copia-cli/pkg/cmd/issue/edit"
	issuelist "github.com/qubernetic/copia-cli/pkg/cmd/issue/list"
	issuepin "github.com/qubernetic/copia-cli/pkg/cmd/issue/pin"
	issuereopen "github.com/qubernetic/copia-cli/pkg/cmd/issue/reopen"
	issueunpin "github.com/qubernetic/copia-cli/pkg/cmd/issue/unpin"
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
	assertOutputContains(t, createOut.String(), "Created issue #")

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
	assertOutputContains(t, viewOut.String(), "[integration-test] CLI lifecycle test")

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

	// Close
	closeIO, closeOut, _ := testIO()
	err = issueclose.CloseRun(&issueclose.CloseOptions{
		IO: closeIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
	})
	require.NoError(t, err)
	assertOutputContains(t, closeOut.String(), "Closed issue")

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
	assertOutputEqual(t, reopenOut.String(), fmt.Sprintf("Reopened issue #%d\n", issueNumber))

	// Reopen again: the issue is open, so nothing is sent
	againIO, againOut, againErr := testIO()
	err = issuereopen.ReopenRun(&issuereopen.ReopenOptions{
		IO: againIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
	})
	require.NoError(t, err)
	assertOutputEmpty(t, againOut.String())
	assertOutputContains(t, againErr.String(), "is already open")
}

func TestIssue_List(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

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
	assert.ErrorContains(t, err, "not found")
}

func TestIssue_Delete(t *testing.T) {
	env := loadTestEnv(t)

	createIO, createOut, _ := testIO()
	err := issuecreate.CreateRun(&issuecreate.CreateOptions{
		IO: createIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Title: "[integration-test] CLI delete test",
		Body:  "Created by integration test for DeleteRun.",
	})
	require.NoError(t, err)

	var issueNumber int64
	_, _ = fmt.Sscanf(createOut.String(), "Created issue #%d", &issueNumber)
	require.Greater(t, issueNumber, int64(0), "failed to parse issue number")

	defer func() {
		closeIO, _, _ := testIO()
		_ = issueclose.CloseRun(&issueclose.CloseOptions{
			IO: closeIO, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: issueNumber,
		})
	}()

	deleteOpts := func(confirmed bool) (*issuedelete.DeleteOptions, *bytes.Buffer) {
		ios, out, _ := testIO()
		return &issuedelete.DeleteOptions{
			IO: ios, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number:    issueNumber,
			Confirmed: confirmed,
		}, out
	}

	// Without --yes nothing is sent and the issue stays
	opts, out := deleteOpts(false)
	err = issuedelete.DeleteRun(opts)
	assert.ErrorContains(t, err, "use --yes to confirm")
	assertOutputEmpty(t, out.String())

	// app.copia.io answers the DELETE with HTTP 405, so the issue stays and
	// the deferred cleanup closes it. Should Copia allow deletion, this fails
	// and docs/parity.md needs updating.
	opts, out = deleteOpts(true)
	err = issuedelete.DeleteRun(opts)
	assert.ErrorContains(t, err, "does not allow deleting issues (HTTP 405)")
	assertOutputEmpty(t, out.String())

	viewIO, _, _ := testIO()
	err = issueview.ViewRun(&issueview.ViewOptions{
		IO: viewIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: issueNumber,
	})
	require.NoError(t, err, "the refused issue still exists")
}

func TestIssue_PinUnpin(t *testing.T) {
	env := loadTestEnv(t)

	const title = "[integration-test] CLI pin test"
	createIO, createOut, _ := testIO()
	err := issuecreate.CreateRun(&issuecreate.CreateOptions{
		IO: createIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Title: title,
		Body:  "Created by integration test for PinRun and UnpinRun.",
	})
	require.NoError(t, err)

	var issueNumber int64
	_, _ = fmt.Sscanf(createOut.String(), "Created issue #%d", &issueNumber)
	require.Greater(t, issueNumber, int64(0), "failed to parse issue number")

	// Cleanup: unpin (a no-op when the test got that far) and close
	defer func() {
		unpinIO, _, _ := testIO()
		_ = issueunpin.UnpinRun(&issueunpin.UnpinOptions{
			IO: unpinIO, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: issueNumber,
		})
		closeIO, _, _ := testIO()
		_ = issueclose.CloseRun(&issueclose.CloseOptions{
			IO: closeIO, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: issueNumber,
		})
	}()

	suffix := fmt.Sprintf("#%d (%s)", issueNumber, title)
	pin := func() (string, string, error) {
		ios, out, errOut := testIO()
		err := issuepin.PinRun(&issuepin.PinOptions{
			IO: ios, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: issueNumber,
		})
		return out.String(), errOut.String(), err
	}
	unpin := func() (string, string, error) {
		ios, out, errOut := testIO()
		err := issueunpin.UnpinRun(&issueunpin.UnpinOptions{
			IO: ios, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: issueNumber,
		})
		return out.String(), errOut.String(), err
	}

	out, errOut, err := pin()
	require.NoError(t, err)
	assertOutputContains(t, out, "Pinned issue ")
	assertOutputContains(t, out, suffix+"\n")
	assertOutputEmpty(t, errOut)

	// Pinning again reads pin_order and sends nothing
	out, errOut, err = pin()
	require.NoError(t, err)
	assertOutputEmpty(t, out)
	assertOutputContains(t, errOut, suffix+" is already pinned\n")

	out, errOut, err = unpin()
	require.NoError(t, err)
	assertOutputContains(t, out, "Unpinned issue ")
	assertOutputContains(t, out, suffix+"\n")
	assertOutputEmpty(t, errOut)

	out, errOut, err = unpin()
	require.NoError(t, err)
	assertOutputEmpty(t, out)
	assertOutputContains(t, errOut, suffix+" is not pinned\n")
}
