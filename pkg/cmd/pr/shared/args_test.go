package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePRArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want PRRef
	}{
		{name: "number", arg: "7", want: PRRef{Number: 7}},
		{name: "hash number", arg: "#7", want: PRRef{Number: 7}},
		{
			name: "url",
			arg:  "https://app.copia.io/my-org/my-repo/pulls/7",
			want: PRRef{Host: "app.copia.io", Owner: "my-org", Repo: "my-repo", Number: 7},
		},
		{
			name: "url with trailing slash and fragment",
			arg:  "https://app.copia.io/my-org/my-repo/pulls/7/#issuecomment-3",
			want: PRRef{Host: "app.copia.io", Owner: "my-org", Repo: "my-repo", Number: 7},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePRArg(tt.arg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParsePRArg_Invalid(t *testing.T) {
	for _, arg := range []string{
		"abc",
		"0",
		"-3",
		"https://app.copia.io/my-org/my-repo/issues/7",
		"https://app.copia.io/my-org/my-repo/pulls/abc",
		"https://app.copia.io/my-org/pulls/7",
		"http://app.copia.io/my-org/my-repo/pulls/7",
		"https://app.copia.io/a/my-org/my-repo/pulls/7",
		"https://%zz",
	} {
		t.Run(arg, func(t *testing.T) {
			_, err := ParsePRArg(arg)
			require.EqualError(t, err, "invalid pull request number or URL: "+arg)
		})
	}
}

func TestPRRef_Resolve(t *testing.T) {
	ref := PRRef{Number: 7}
	owner, repo, err := ref.Resolve("app.copia.io", "my-org", "my-repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"my-org", "my-repo"}, []string{owner, repo})

	ref = PRRef{Host: "app.copia.io", Owner: "other", Repo: "thing", Number: 7}
	owner, repo, err = ref.Resolve("APP.copia.io", "my-org", "my-repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"other", "thing"}, []string{owner, repo})

	ref = PRRef{Host: "copia.example.com", Owner: "other", Repo: "thing", Number: 7}
	_, _, err = ref.Resolve("app.copia.io", "my-org", "my-repo")
	require.EqualError(t, err, "pull request URL host copia.example.com does not match the active host app.copia.io")
}
