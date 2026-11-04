//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	prclose "github.com/qubernetic/copia-cli/pkg/cmd/pr/close"
	prcomment "github.com/qubernetic/copia-cli/pkg/cmd/pr/comment"
	predit "github.com/qubernetic/copia-cli/pkg/cmd/pr/edit"
	prlist "github.com/qubernetic/copia-cli/pkg/cmd/pr/list"
	prready "github.com/qubernetic/copia-cli/pkg/cmd/pr/ready"
	prreopen "github.com/qubernetic/copia-cli/pkg/cmd/pr/reopen"
	prview "github.com/qubernetic/copia-cli/pkg/cmd/pr/view"
)

func TestPR_List(t *testing.T) {
	env := loadTestEnv(t)
	ios, _, _ := testIO()

	opts := &prlist.ListOptions{
		IO:         ios,
		HTTPClient: &http.Client{},
		Host:       env.Host,
		Token:      env.Token,
		Owner:      env.Owner,
		Repo:       env.Repo,
		State:      "all",
		Limit:      5,
	}

	err := prlist.ListRun(opts)
	require.NoError(t, err)
}

func TestPR_CloseReopen(t *testing.T) {
	env := loadTestEnv(t)

	const title = "[integration-test] CLI reopen test"
	pr := createFixturePR(t, env, title)

	closeIO, closeOut, _ := testIO()
	err := prclose.CloseRun(&prclose.CloseOptions{
		IO: closeIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: pr.Number,
	})
	require.NoError(t, err)
	assertOutputEqual(t, closeOut.String(), fmt.Sprintf("Closed PR #%d\n", pr.Number))

	reopen := func(comment string) (string, string, error) {
		ios, out, errOut := testIO()
		err := prreopen.ReopenRun(&prreopen.ReopenOptions{
			IO: ios, HTTPClient: &http.Client{},
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number:  pr.Number,
			Comment: comment,
		})
		return out.String(), errOut.String(), err
	}
	ref := fmt.Sprintf("%s/%s#%d (%s)", env.Owner, env.Repo, pr.Number, title)

	out, errOut, err := reopen("Reopened by integration test.")
	require.NoError(t, err)
	assertOutputEqual(t, out, "Reopened pull request "+ref+"\n")
	assertOutputEmpty(t, errOut)

	status, data := apiCall(t, env, "read pull request", "GET",
		fmt.Sprintf("/repos/%s/%s/pulls/%d", env.Owner, env.Repo, pr.Number), nil)
	assertStatusCode(t, status, http.StatusOK)
	var state struct {
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(data, &state))
	require.True(t, state.State == "open", "pull request is open after reopen")

	// An open pull request is left as it is
	out, errOut, err = reopen("")
	require.NoError(t, err)
	assertOutputEmpty(t, out)
	assertOutputEqual(t, errOut, "! Pull request "+ref+" is already open\n")
}

func TestPR_Comment(t *testing.T) {
	env := loadTestEnv(t)
	pr := createFixturePR(t, env, "[integration-test] CLI comment test")

	body := "Comment by integration test " + strconv.FormatInt(time.Now().UnixNano(), 36)
	ios, out, _ := testIO()
	err := prcomment.CommentRun(&prcomment.CommentOptions{
		IO: ios, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: pr.Number,
		Body:   body,
	})
	require.NoError(t, err)
	assertOutputContains(t, out.String(), "#issuecomment-")

	_, id, _ := strings.Cut(strings.TrimSpace(out.String()), "#issuecomment-")
	t.Cleanup(func() {
		status, _ := apiCall(t, env, "delete comment", "DELETE",
			fmt.Sprintf("/repos/%s/%s/issues/comments/%s", env.Owner, env.Repo, id), nil)
		assertStatusCode(t, status, http.StatusNoContent)
	})

	viewIO, viewOut, _ := testIO()
	err = prview.ViewRun(&prview.ViewOptions{
		IO: viewIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number:   pr.Number,
		Comments: true,
	})
	require.NoError(t, err)
	assertOutputContains(t, viewOut.String(), body)
}

func TestPR_Edit(t *testing.T) {
	env := loadTestEnv(t)
	pr := createFixturePR(t, env, "[integration-test] CLI edit test")
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)

	label := "it-pr-edit-" + suffix
	status, data := apiCall(t, env, "create label", "POST",
		fmt.Sprintf("/repos/%s/%s/labels", env.Owner, env.Repo), map[string]string{"name": label, "color": "#00aabb"})
	if !assertStatusCode(t, status, http.StatusCreated) {
		t.FailNow()
	}
	var created struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &created))
	t.Cleanup(func() {
		status, _ := apiCall(t, env, "delete label", "DELETE",
			fmt.Sprintf("/repos/%s/%s/labels/%d", env.Owner, env.Repo, created.ID), nil)
		assertStatusCode(t, status, http.StatusNoContent)
	})

	title := "[integration-test] CLI edit test, edited " + suffix
	body := "Body edited by integration test " + suffix
	ios, out, _ := testIO()
	err := predit.EditRun(&predit.EditOptions{
		IO: ios, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number:    pr.Number,
		Title:     title,
		TitleSet:  true,
		Body:      &body,
		AddLabels: []string{label},
	})
	require.NoError(t, err)
	assertOutputContains(t, out.String(), fmt.Sprintf("/pulls/%d", pr.Number))

	viewIO, viewOut, _ := testIO()
	err = prview.ViewRun(&prview.ViewOptions{
		IO: viewIO, HTTPClient: &http.Client{},
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Number: pr.Number,
	})
	require.NoError(t, err)
	assertOutputContains(t, viewOut.String(), fmt.Sprintf("#%d %s\n", pr.Number, title))
	assertOutputContains(t, viewOut.String(), "\n"+body+"\n")
	assertOutputContains(t, viewOut.String(), "Labels: "+label+"\n")
}

func TestPR_Ready(t *testing.T) {
	env := loadTestEnv(t)
	title := "[integration-test] CLI ready test " + strconv.FormatInt(time.Now().UnixNano(), 36)
	pr := createDraftFixturePR(t, env, title)
	ref := fmt.Sprintf("%s/%s#%d", env.Owner, env.Repo, pr.Number)

	ready := func(undo bool) (string, string) {
		t.Helper()
		ios, out, errOut := testIO()
		err := prready.ReadyRun(&prready.ReadyOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: pr.Number,
			Undo:   undo,
		})
		require.NoError(t, err)
		return out.String(), errOut.String()
	}
	viewTitle := func() string {
		t.Helper()
		ios, out, _ := testIO()
		err := prview.ViewRun(&prview.ViewOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: pr.Number,
		})
		require.NoError(t, err)
		return out.String()
	}
	assertOutputContains(t, viewTitle(), fmt.Sprintf("#%d WIP: %s\n", pr.Number, title))

	out, errOut := ready(false)
	assertOutputEqual(t, out, fmt.Sprintf("Pull request %s (%s) is marked as \"ready for review\"\n", ref, title))
	assertOutputEmpty(t, errOut)
	assertOutputContains(t, viewTitle(), fmt.Sprintf("#%d %s\n", pr.Number, title))

	out, errOut = ready(false)
	assertOutputEmpty(t, out)
	assertOutputEqual(t, errOut, fmt.Sprintf("! Pull request %s (%s) is already \"ready for review\"\n", ref, title))

	out, errOut = ready(true)
	assertOutputEqual(t, out, fmt.Sprintf("Pull request %s (WIP: %s) is converted to \"draft\"\n", ref, title))
	assertOutputEmpty(t, errOut)
	assertOutputContains(t, viewTitle(), fmt.Sprintf("#%d WIP: %s\n", pr.Number, title))
}
