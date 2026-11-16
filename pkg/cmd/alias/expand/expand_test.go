package expand

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandAlias(t *testing.T) {
	aliases := map[string]string{
		"il":     "issue list -L 1",
		"co":     "pr checkout",
		"ilabel": "issue list --label $1 --state $2",
		"quoted": `issue create --title "fix $1" --body "a b"`,
		"twice":  "issue view $1 --comments -R $1",
		"ten":    "issue list $10",
		"igrep":  "!copia-cli issue list --label $1 | grep $2",
		"sh":     "!echo hi",
		"broken": `issue list "unclosed`,
	}

	tests := []struct {
		name      string
		args      []string
		want      []string
		wantShell bool
		wantErr   string
	}{
		{name: "no args", args: nil, want: nil},
		{name: "not an alias", args: []string{"issue", "list"}, want: []string{"issue", "list"}},
		{name: "a flag", args: []string{"--version"}, want: []string{"--version"}},
		{name: "expansion", args: []string{"il"}, want: []string{"issue", "list", "-L", "1"}},
		{
			name: "extra arguments are appended",
			args: []string{"il", "-R", "owner/repo", "--state", "all"},
			want: []string{"issue", "list", "-L", "1", "-R", "owner/repo", "--state", "all"},
		},
		{name: "extra argument", args: []string{"co", "12"}, want: []string{"pr", "checkout", "12"}},
		{
			name: "placeholders",
			args: []string{"ilabel", "bug", "closed"},
			want: []string{"issue", "list", "--label", "bug", "--state", "closed"},
		},
		{
			name: "placeholders take the arguments, extra ones are dropped",
			args: []string{"ilabel", "bug", "closed", "extra"},
			want: []string{"issue", "list", "--label", "bug", "--state", "closed"},
		},
		{
			name: "an argument is never split again",
			args: []string{"ilabel", "needs review", `x" --token "y`},
			want: []string{"issue", "list", "--label", "needs review", "--state", `x" --token "y`},
		},
		{
			name: "a placeholder in an argument is kept",
			args: []string{"ilabel", "$2", "closed"},
			want: []string{"issue", "list", "--label", "$2", "--state", "closed"},
		},
		{
			name: "a placeholder inside a quoted word",
			args: []string{"quoted", "the build"},
			want: []string{"issue", "create", "--title", "fix the build", "--body", "a b"},
		},
		{
			name: "a placeholder used twice",
			args: []string{"twice", "7"},
			want: []string{"issue", "view", "7", "--comments", "-R", "7"},
		},
		{
			name: "a two-digit placeholder",
			args: []string{"ten", "1", "2", "3", "4", "5", "6", "7", "8", "9", "ten"},
			want: []string{"issue", "list", "ten"},
		},
		{name: "missing placeholder argument", args: []string{"ilabel", "bug"}, wantErr: "not enough arguments for alias: issue list --label $1 --state $2"},
		{name: "unparsable expansion", args: []string{"broken"}, wantErr: "alias broken: unterminated double quote"},
		{name: "shell alias", args: []string{"sh"}, want: []string{"-c", "echo hi"}, wantShell: true},
		{
			name:      "shell alias passes the arguments as positional parameters",
			args:      []string{"igrep", "bug", "$(rm -rf /)"},
			want:      []string{"-c", "copia-cli issue list --label $1 | grep $2", "--", "bug", "$(rm -rf /)"},
			wantShell: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isShell, err := ExpandAlias(aliases, tt.args)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantShell, isShell)
		})
	}
}
