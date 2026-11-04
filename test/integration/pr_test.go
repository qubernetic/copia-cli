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
	prlist "github.com/qubernetic/copia-cli/pkg/cmd/pr/list"
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
