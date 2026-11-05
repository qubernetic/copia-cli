//go:build integration

package integration

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	prstatus "github.com/qubernetic/copia-cli/pkg/cmd/pr/status"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func TestPR_Status(t *testing.T) {
	env := loadTestEnv(t)

	// The test account opens the pull request, so it is the pull request of
	// its head branch and one of the account's own.
	fx := createFixturePR(t, env, "[integration-test] pr status")

	statusOpts := func() (*prstatus.StatusOptions, func() string) {
		ios, out, _ := testIO()
		return &prstatus.StatusOptions{
			IO: ios, HTTPClient: testHTTPClient(),
			Host: env.Host, Token: env.Token,
			Owner: env.Owner, Repo: env.Repo,
			Branch: func() (string, error) { return fx.Head, nil },
		}, out.String
	}

	opts, out := statusOpts()
	require.NoError(t, prstatus.StatusRun(opts), "pr status")
	assertOutputContains(t, out(), "Current branch")
	assertOutputContains(t, out(), "Created by you")
	assertOutputContains(t, out(), "Requesting a code review from you")
	assertOutputContains(t, out(), "[integration-test] pr status")

	type entry struct {
		Number int64 `json:"number"`
	}
	opts, out = statusOpts()
	opts.JSON = cmdutil.JSONFlags{Fields: []string{"number"}}
	require.NoError(t, prstatus.StatusRun(opts), "pr status --json")
	var sections struct {
		CurrentBranch *entry  `json:"currentBranch"`
		CreatedBy     []entry `json:"createdBy"`
		NeedsReview   []entry `json:"needsReview"`
	}
	require.NoError(t, json.Unmarshal([]byte(out()), &sections), "decode pr status --json")
	isFixture := func(e entry) bool { return e.Number == fx.Number }
	require.True(t, sections.CurrentBranch != nil && isFixture(*sections.CurrentBranch), "the fixture is the pull request of its branch")
	require.True(t, slices.ContainsFunc(sections.CreatedBy, isFixture), "the fixture is in createdBy")
	// Nobody was asked to review the fixture: if Copia ignored
	// review_requested, the search would list it here.
	require.False(t, slices.ContainsFunc(sections.NeedsReview, isFixture), "the fixture is not in needsReview")

	// A branch without a pull request.
	opts, out = statusOpts()
	opts.Branch = func() (string, error) { return fx.Head + "-none", nil }
	opts.JSON = cmdutil.JSONFlags{Fields: []string{"number"}}
	require.NoError(t, prstatus.StatusRun(opts), "pr status on a branch without a pull request")
	var none struct {
		CurrentBranch *entry `json:"currentBranch"`
	}
	require.NoError(t, json.Unmarshal([]byte(out()), &none), "decode pr status --json without a pull request")
	require.True(t, none.CurrentBranch == nil, "a branch without a pull request has none")
}
