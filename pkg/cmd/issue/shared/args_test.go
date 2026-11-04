package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIssueArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want IssueRef
	}{
		{name: "number", arg: "12", want: IssueRef{Number: 12}},
		{name: "hash number", arg: "#12", want: IssueRef{Number: 12}},
		{
			name: "url",
			arg:  "https://app.copia.io/my-org/my-repo/issues/12",
			want: IssueRef{Host: "app.copia.io", Owner: "my-org", Repo: "my-repo", Number: 12},
		},
		{
			name: "url with trailing slash and fragment",
			arg:  "https://app.copia.io/my-org/my-repo/issues/12/#issuecomment-3",
			want: IssueRef{Host: "app.copia.io", Owner: "my-org", Repo: "my-repo", Number: 12},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIssueArg(tt.arg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseIssueArg_Invalid(t *testing.T) {
	for _, arg := range []string{
		"abc",
		"0",
		"-3",
		"https://app.copia.io/my-org/my-repo/pulls/12",
		"https://app.copia.io/my-org/my-repo/issues/abc",
		"https://app.copia.io/my-org/issues/12",
		"http://app.copia.io/my-org/my-repo/issues/12",
		"https://app.copia.io/a/my-org/my-repo/issues/12",
		"https://%zz",
	} {
		t.Run(arg, func(t *testing.T) {
			_, err := ParseIssueArg(arg)
			require.EqualError(t, err, "invalid issue number or URL: "+arg)
		})
	}
}

func TestIssueRef_Resolve(t *testing.T) {
	ref := IssueRef{Number: 12}
	owner, repo, err := ref.Resolve("app.copia.io", "my-org", "my-repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"my-org", "my-repo"}, []string{owner, repo})

	ref = IssueRef{Host: "app.copia.io", Owner: "other", Repo: "thing", Number: 12}
	owner, repo, err = ref.Resolve("APP.copia.io", "my-org", "my-repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"other", "thing"}, []string{owner, repo})

	ref = IssueRef{Host: "copia.example.com", Owner: "other", Repo: "thing", Number: 12}
	_, _, err = ref.Resolve("app.copia.io", "my-org", "my-repo")
	require.EqualError(t, err, "issue URL host copia.example.com does not match the active host app.copia.io")
}
