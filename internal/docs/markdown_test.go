package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noop(cmd *cobra.Command, args []string) error { return nil }

func render(t *testing.T, cmd *cobra.Command) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, genMarkdown(cmd, &buf))
	return buf.String()
}

func TestGenMarkdown_TitleAndUsage(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Short: "Test command", RunE: noop}
	cmd.Flags().Bool("web", false, "Open in browser")

	out := render(t, cmd)

	assert.True(t, strings.HasPrefix(out, "# test\n"))
	assert.Contains(t, out, "```text\ntest [flags]\n```")
}

func TestGenMarkdown_NoJekyll(t *testing.T) {
	root := &cobra.Command{Use: "test", Short: "Root"}
	root.PersistentFlags().String("host", "", "Copia host")
	child := &cobra.Command{
		Use:     "child",
		Short:   "Child",
		Aliases: []string{"c"},
		Example: "test child",
		RunE:    noop,
	}
	child.Flags().StringP("state", "s", "open", "Filter by state")
	grand := &cobra.Command{Use: "grand", Short: "Grandchild", RunE: noop}
	root.AddCommand(child)
	child.AddCommand(grand)

	for _, cmd := range []*cobra.Command{root, child, grand} {
		out := render(t, cmd)
		for _, s := range []string{"{%", "%}", "<dl", "layout:", "permalink:"} {
			assert.NotContains(t, out, s)
		}
		assert.False(t, strings.HasPrefix(out, "---"))
	}
}

func TestGenMarkdown_NoBranding(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "copia-cli",
		Short: "Copia CLI",
		Long:  "Work with Copia repositories.",
	}

	out := render(t, cmd)

	assert.NotContains(t, out, "github.com")
	assert.NotContains(t, out, "GitHub Enterprise")
	assert.NotContains(t, out, " gh ")
}

func TestGenMarkdown_OptionsAreADefinitionList(t *testing.T) {
	cmd := &cobra.Command{Use: "list", Short: "List items", RunE: noop}
	cmd.Flags().StringP("state", "s", "open", "Filter by state")
	cmd.Flags().Int("limit", 30, "Maximum")
	cmd.Flags().Bool("web", false, "Open in browser")

	out := render(t, cmd)

	assert.Contains(t, out, "### Options\n\n")
	assert.Contains(t, out, "`-s`, `--state <string>`\n:   Filter by state (default \"open\")\n")
	assert.Contains(t, out, "`--limit <int>`\n:   Maximum (default 30)\n")
	assert.Contains(t, out, "`--web`\n:   Open in browser\n")
	assert.NotContains(t, out, "--help")
}

func TestGenMarkdown_InheritedOptions(t *testing.T) {
	root := &cobra.Command{Use: "test", Short: "Root"}
	root.PersistentFlags().String("host", "", "Copia host")
	child := &cobra.Command{Use: "child", Short: "Child", RunE: noop}
	root.AddCommand(child)

	out := render(t, child)

	assert.Contains(t, out, "### Options inherited from parent commands\n\n`--host <string>`\n:   Copia host\n")
}

func TestGenMarkdown_MultiLineFlagUsageStaysInItsDefinition(t *testing.T) {
	cmd := &cobra.Command{Use: "list", Short: "List items", RunE: noop}
	cmd.Flags().String("first", "", "First line\nsecond line")
	cmd.Flags().String("second", "", "Another flag")

	out := render(t, cmd)

	assert.Contains(t, out, ":   First line\n    second line\n")
	assert.Contains(t, out, "`--second <string>`\n:   Another flag\n")
}

func TestGenMarkdown_AngleBracketsStayVisible(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "close",
		Long: "Close <number> in <owner>/<repo>. Use `--repo <name>`.",
		RunE: noop,
	}
	cmd.Flags().String("file", "", "Take <file>")

	out := render(t, cmd)

	assert.Contains(t, out, "Close &lt;number> in &lt;owner>/&lt;repo>.")
	assert.Contains(t, out, "`--repo <name>`")
	assert.Contains(t, out, ":   Take &lt;file>")
}

func TestGenMarkdown_AngleBracketsInADefaultStayVisible(t *testing.T) {
	cmd := &cobra.Command{Use: "list", Short: "List items", RunE: noop}
	cmd.Flags().String("sort", "<none>", "Sort by <field>")

	out := render(t, cmd)

	assert.Contains(t, out, ":   Sort by &lt;field> (default \"&lt;none>\")\n")
}

func TestEscapeAngles(t *testing.T) {
	assert.Equal(t, "a &lt;b> `<c>` d", escapeAngles("a <b> `<c>` d"))
	assert.Equal(t, "```\n<x>\n```\n&lt;y>", escapeAngles("```\n<x>\n```\n<y>"))
}

func TestGenMarkdown_DescriptionFallsBackToShort(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Short: "Only short"}

	out := render(t, cmd)

	assert.Equal(t, "# test\n\nOnly short\n", out)
}

func TestGenMarkdown_ExamplesAreAFencedBlockWithoutIndent(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "close",
		Short:   "Close an issue",
		RunE:    noop,
		Example: "  # Close an issue\n  copia issue close 12\n    --comment done\n",
	}

	out := render(t, cmd)

	assert.Contains(t, out, "### Examples\n\n```bash\n# Close an issue\ncopia issue close 12\n  --comment done\n```\n")
}

func TestGenMarkdown_AliasesAreCode(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	parent := &cobra.Command{Use: "issue", Short: "Manage issues"}
	child := &cobra.Command{Use: "list", Short: "List issues", Aliases: []string{"ls", "l"}, RunE: noop}
	root.AddCommand(parent)
	parent.AddCommand(child)

	out := render(t, child)

	assert.Contains(t, out, "### Aliases\n\n`test issue l`, `test issue ls`\n")
}

func TestGenMarkdown_JSONFields(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List items",
		RunE:        noop,
		Annotations: map[string]string{"help:json-fields": "number,title,state,labels"},
	}

	out := render(t, cmd)

	assert.Contains(t, out, "### JSON Fields\n\n`labels`, `number`, `state`, `title`\n")
}

func TestGenMarkdown_SubcommandGroups(t *testing.T) {
	parent := &cobra.Command{Use: "issue", Short: "Manage issues"}
	parent.AddGroup(&cobra.Group{ID: "general", Title: "General commands"})
	parent.AddGroup(&cobra.Group{ID: "targeted", Title: "Targeted commands"})

	create := &cobra.Command{Use: "create", Short: "Create an issue", GroupID: "general", RunE: noop}
	list := &cobra.Command{Use: "list", Short: "List issues", GroupID: "general", RunE: noop}
	closeCmd := &cobra.Command{Use: "close", Short: "Close an issue", GroupID: "targeted", RunE: noop}
	view := &cobra.Command{Use: "view", Short: "View an issue", GroupID: "targeted", RunE: noop}
	parent.AddCommand(create, list, closeCmd, view)

	out := render(t, parent)

	assert.Contains(t, out, "### General commands")
	assert.Contains(t, out, "### Targeted commands")
	assert.True(t, strings.Index(out, "General commands") < strings.Index(out, "Targeted commands"))
}

func TestGenMarkdown_LinksAreFileNames(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	parent := &cobra.Command{Use: "issue", Short: "Manage issues"}
	child := &cobra.Command{Use: "close", Short: "Close an issue", RunE: noop}
	root.AddCommand(parent)
	parent.AddCommand(child)

	parentOut := render(t, root)
	childOut := render(t, parent)

	assert.Contains(t, parentOut, "* [test issue](test_issue.md)\n")
	assert.Contains(t, childOut, "### See also\n\n* [test](test.md)\n")
}

func TestGenMarkdownTree_WritesOneFilePerVisibleCommand(t *testing.T) {
	root := &cobra.Command{Use: "test", Short: "Root"}
	root.AddCommand(
		&cobra.Command{Use: "child", Short: "Child", RunE: noop},
		&cobra.Command{Use: "hidden", Short: "Hidden", Hidden: true, RunE: noop},
		&cobra.Command{
			Use: "forced", Short: "Forced", Hidden: true, RunE: noop,
			Annotations: map[string]string{"markdown:generate": "true"},
		},
	)
	dir := t.TempDir()

	require.NoError(t, GenMarkdownTree(root, dir))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(body), "# "), e.Name())
	}
	sort.Strings(names)
	assert.Equal(t, []string{"test.md", "test_child.md", "test_forced.md"}, names)
}

func TestCmdManualPath(t *testing.T) {
	parent := &cobra.Command{Use: "copia-cli"}
	child := &cobra.Command{Use: "issue"}
	sub := &cobra.Command{Use: "close"}
	parent.AddCommand(child)
	child.AddCommand(sub)

	assert.Equal(t, "copia-cli_issue_close.md", cmdManualPath(sub))
}

func TestFormatSlice(t *testing.T) {
	result := formatSlice([]string{"b", "a", "c"}, "`", "`", true)
	assert.Contains(t, result, "`a`")
	assert.Contains(t, result, "`b`")
	assert.Contains(t, result, "`c`")
}
