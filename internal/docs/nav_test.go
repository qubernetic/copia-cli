package docs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCmd(use string, children ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{Use: use, Short: use, RunE: noop}
	c.AddCommand(children...)
	return c
}

func navTree() *cobra.Command {
	return newCmd("copia-cli",
		newCmd("repo", newCmd("deploy-key", newCmd("add"))),
		newCmd("status"),
		newCmd("issue", newCmd("list"), newCmd("close")),
		newCmd("api"),
	)
}

func nav(title string, value any) map[string]any { return map[string]any{title: value} }

func TestCommandsNav_LeavesThenGroups(t *testing.T) {
	want := []any{
		nav("Overview", "manual/copia-cli.md"),
		nav("api", "manual/copia-cli_api.md"),
		nav("status", "manual/copia-cli_status.md"),
		nav("issue", []any{
			nav("issue", "manual/copia-cli_issue.md"),
			nav("issue close", "manual/copia-cli_issue_close.md"),
			nav("issue list", "manual/copia-cli_issue_list.md"),
		}),
		nav("repo", []any{
			nav("repo", "manual/copia-cli_repo.md"),
			nav("repo deploy-key", []any{
				nav("repo deploy-key", "manual/copia-cli_repo_deploy-key.md"),
				nav("repo deploy-key add", "manual/copia-cli_repo_deploy-key_add.md"),
			}),
		}),
	}

	assert.Equal(t, want, CommandsNav(navTree(), "manual/"))
}

func TestCommandsNav_SkipsHiddenKeepsForced(t *testing.T) {
	hidden := newCmd("secret")
	hidden.Hidden = true
	forced := newCmd("forced")
	forced.Hidden = true
	forced.Annotations = map[string]string{"markdown:generate": "true"}
	onlyHidden := newCmd("group", func() *cobra.Command { h := newCmd("inner"); h.Hidden = true; return h }())
	root := newCmd("copia-cli", hidden, forced, onlyHidden)

	want := []any{
		nav("Overview", "manual/copia-cli.md"),
		nav("forced", "manual/copia-cli_forced.md"),
		nav("group", "manual/copia-cli_group.md"),
	}

	assert.Equal(t, want, CommandsNav(root, "manual/"))
}

func navPaths(items []any, prefix string, out *[]string) {
	for _, item := range items {
		for _, v := range item.(map[string]any) {
			switch v := v.(type) {
			case string:
				*out = append(*out, strings.TrimPrefix(v, prefix))
			case []any:
				navPaths(v, prefix, out)
			}
		}
	}
}

func TestCommandsNav_MatchesTheWrittenPages(t *testing.T) {
	hidden := newCmd("secret", newCmd("child"))
	hidden.Hidden = true
	forced := newCmd("forced")
	forced.Hidden = true
	forced.Annotations = map[string]string{"markdown:generate": "true"}
	root := navTree()
	root.AddCommand(hidden, forced, newCmd("renamed"))
	root.Commands()[len(root.Commands())-1].Annotations = map[string]string{"markdown:basename": "custom-name"}

	dir := t.TempDir()
	require.NoError(t, GenMarkdownTree(root, dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var written []string
	for _, e := range entries {
		written = append(written, e.Name())
	}

	var inNav []string
	navPaths(CommandsNav(root, "manual/"), "manual/", &inNav)

	sort.Strings(written)
	sort.Strings(inNav)
	assert.Equal(t, written, inNav)
}

func TestWriteNav_IsJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands-nav.json")
	require.NoError(t, WriteNav(navTree(), path, "manual/"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(raw), "}\n]\n"))

	var got []any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, CommandsNav(navTree(), "manual/"), got)
}
