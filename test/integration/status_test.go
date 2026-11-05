//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	issueclose "github.com/qubernetic/copia-cli/pkg/cmd/issue/close"
	issuecreate "github.com/qubernetic/copia-cli/pkg/cmd/issue/create"
	"github.com/qubernetic/copia-cli/pkg/cmd/status"
)

func TestStatus(t *testing.T) {
	env := loadTestEnv(t)

	// An issue the test account opens and assigns to itself is the newest
	// of its assigned issues.
	createIO, createOut, _ := testIO()
	err := issuecreate.CreateRun(&issuecreate.CreateOptions{
		IO: createIO, HTTPClient: testHTTPClient(),
		Host: env.Host, Token: env.Token,
		Owner: env.Owner, Repo: env.Repo,
		Title:     "[integration-test] status",
		Body:      "Created by the status integration test.",
		Assignees: []string{"@me"},
	})
	require.NoError(t, err, "create issue")
	var number int64
	_, _ = fmt.Sscanf(createOut.String(), "Created issue #%d", &number)
	require.Greater(t, number, int64(0), "failed to parse issue number")
	defer func() {
		closeIO, _, _ := testIO()
		_ = issueclose.CloseRun(&issueclose.CloseOptions{
			IO: closeIO, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Number: number,
		})
	}()

	run := func(exclude ...string) string {
		ios, out, _ := testIO()
		// A wide terminal, so no title is cut.
		ios.SetStdoutTTY(true)
		ios.SetTerminalWidth(400)
		require.NoError(t, status.StatusRun(&status.StatusOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Org:     env.Owner,
			Exclude: exclude,
		}), "status")
		return out.String()
	}

	// The identifier is followed by the column gap, so #12 does not match #123.
	row := fmt.Sprintf("%s/%s#%d  ", env.Owner, env.Repo, number)

	out := run()
	for _, header := range []string{"Assigned Issues", "Assigned Pull Requests", "Review Requests", "Mentions", "Repository Activity"} {
		assertOutputContains(t, out, header)
	}
	assertOutputContains(t, out, "[integration-test] status")
	require.True(t, strings.Contains(out, row), "the new issue is listed")
	// Assigned Issues is the left column of the first pair, so its rows
	// start a line; Assigned Pull Requests follows the vertical line.
	assigned, _, _ := strings.Cut(out, "Review Requests")
	require.True(t, strings.Contains(assigned, "\n"+row), "the new issue is in Assigned Issues")

	out = run(strings.ToUpper(env.Owner + "/" + env.Repo))
	require.False(t, strings.Contains(strings.ToLower(out), strings.ToLower(row)), "--exclude drops the repository regardless of case")
}
