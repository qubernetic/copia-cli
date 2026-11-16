// Package docs generates MkDocs-compatible Markdown documentation for Cobra commands.
// Adapted from github.com/cli/cli/v2/internal/docs with GitHub-specific content removed.
//
//nolint:errcheck // fmt.Fprint write errors are intentionally ignored, matching gh CLI upstream style.
package docs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// formatSlice concatenates elements into a comma-separated string.
// Elements are wrapped with prepend/append strings. If doSort is true, values are sorted.
func formatSlice(values []string, prependWith, appendWith string, doSort bool) string {
	if doSort {
		values = slices.Clone(values)
		slices.Sort(values)
	}

	var parts []string
	for _, v := range values {
		parts = append(parts, prependWith+strings.TrimSpace(v)+appendWith)
	}
	return strings.Join(parts, ", ")
}

// buildAliasList builds full alias paths for a command.
func buildAliasList(cmd *cobra.Command, aliases []string) []string {
	if !cmd.HasParent() {
		return aliases
	}

	parentPath := cmd.Parent().CommandPath()
	var result []string
	for _, a := range aliases {
		result = append(result, parentPath+" "+a)
	}
	return result
}

// CommandGroup represents a group of subcommands with a title.
type CommandGroup struct {
	Title    string
	Commands []*cobra.Command
}

// groupedCommands returns subcommands organized by their Cobra GroupID.
func groupedCommands(cmd *cobra.Command) []CommandGroup {
	var res []CommandGroup

	for _, g := range cmd.Groups() {
		var cmds []*cobra.Command
		for _, c := range cmd.Commands() {
			if c.GroupID == g.ID && c.IsAvailableCommand() {
				cmds = append(cmds, c)
			}
		}
		if len(cmds) > 0 {
			res = append(res, CommandGroup{
				Title:    g.Title,
				Commands: cmds,
			})
		}
	}

	var cmds []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.GroupID == "" && c.IsAvailableCommand() {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) > 0 {
		defaultTitle := "Additional commands"
		if len(cmd.Groups()) == 0 {
			defaultTitle = "Available commands"
		}
		res = append(res, CommandGroup{
			Title:    defaultTitle,
			Commands: cmds,
		})
	}

	return res
}

// section is one block of a page. Blocks are joined by a blank line, and an
// empty block is left out.
type section string

func jsonFieldsSection(cmd *cobra.Command) section {
	raw, ok := cmd.Annotations["help:json-fields"]
	if !ok {
		return ""
	}
	return section("### JSON Fields\n\n" + formatSlice(strings.Split(raw, ","), "`", "`", true) + "\n")
}

func aliasesSection(cmd *cobra.Command) section {
	if len(cmd.Aliases) == 0 {
		return ""
	}
	aliases := buildAliasList(cmd, cmd.Aliases)
	return section("### Aliases\n\n" + formatSlice(aliases, "`", "`", true) + "\n")
}

func optionsSections(cmd *cobra.Command) []section {
	var res []section
	if flags := cmd.NonInheritedFlags(); flags.HasAvailableFlags() {
		res = append(res, section("### Options\n\n"+printFlags(flags)))
	}
	if parentFlags := cmd.InheritedFlags(); hasNonHelpFlags(parentFlags) {
		res = append(res, section("### Options inherited from parent commands\n\n"+printFlags(parentFlags)))
	}
	return res
}

func hasNonHelpFlags(fs *pflag.FlagSet) (found bool) {
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			found = true
		}
	})
	return
}

var hiddenFlagDefaults = map[string]bool{
	"false": true,
	"":      true,
	"[]":    true,
	"0s":    true,
}

var defaultValFormats = map[string]string{
	"string":   " (default \"%s\")",
	"duration": " (default \"%s\")",
}

func getDefaultValueDisplayString(f *pflag.Flag) string {
	if hiddenFlagDefaults[f.DefValue] || hiddenFlagDefaults[f.Value.Type()] {
		return ""
	}

	if dvf, found := defaultValFormats[f.Value.Type()]; found {
		return fmt.Sprintf(dvf, f.Value)
	}
	return fmt.Sprintf(" (default %s)", f.Value)
}

// printFlags renders flags as a definition list: the term line holds the
// names, the definition holds the usage text and the default.
func printFlags(fs *pflag.FlagSet) string {
	var entries []string
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		varname, usage := pflag.UnquoteUsage(f)

		var term strings.Builder
		if f.Shorthand != "" {
			fmt.Fprintf(&term, "`-%s`, ", f.Shorthand)
		}
		term.WriteString("`--" + f.Name)
		if varname != "" {
			term.WriteString(" <" + varname + ">")
		}
		term.WriteString("`")

		// Continuation lines are indented to stay inside the definition.
		def := escapeAngles(usage) + getDefaultValueDisplayString(f)
		lines := strings.Split(def, "\n")
		for i := 1; i < len(lines); i++ {
			if lines[i] != "" {
				lines[i] = "    " + lines[i]
			}
		}
		entries = append(entries, term.String()+"\n:   "+strings.Join(lines, "\n")+"\n")
	})
	return strings.Join(entries, "\n")
}

// escapeAngles writes "<" as "&lt;" so that Markdown does not read prose such
// as <number> as an HTML tag. Inline code spans and fenced blocks are left as
// they are, since MkDocs shows their content literally.
func escapeAngles(s string) string {
	lines := strings.Split(s, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		var b strings.Builder
		inCode := false
		for _, r := range line {
			switch {
			case r == '`':
				inCode = !inCode
			case r == '<' && !inCode:
				b.WriteString("&lt;")
				continue
			}
			b.WriteRune(r)
		}
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

// dedent removes the blank lines around text and the leading whitespace its
// lines have in common.
func dedent(s string) string {
	lines := strings.Split(strings.Trim(s, "\n"), "\n")
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range lines {
		if len(l) >= indent && indent > 0 {
			lines[i] = l[indent:]
		} else if strings.TrimSpace(l) == "" {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// genMarkdown writes the page of a single command.
func genMarkdown(cmd *cobra.Command, w io.Writer) error {
	sections := []section{section("# " + cmd.CommandPath() + "\n")}

	if cmd.Runnable() {
		sections = append(sections, section("```text\n"+cmd.UseLine()+"\n```\n"))
	}

	desc := cmd.Long
	if desc == "" {
		desc = cmd.Short
	}
	if desc = strings.TrimSpace(desc); desc != "" {
		sections = append(sections, section(escapeAngles(desc)+"\n"))
	}

	for _, g := range groupedCommands(cmd) {
		var b strings.Builder
		fmt.Fprintf(&b, "### %s\n\n", g.Title)
		for _, subcmd := range g.Commands {
			fmt.Fprintf(&b, "* [%s](%s)\n", subcmd.CommandPath(), cmdManualPath(subcmd))
		}
		sections = append(sections, section(b.String()))
	}

	sections = append(sections, optionsSections(cmd)...)
	sections = append(sections, aliasesSection(cmd), jsonFieldsSection(cmd))

	if example := dedent(cmd.Example); strings.TrimSpace(example) != "" {
		sections = append(sections, section("### Examples\n\n```bash\n"+example+"\n```\n"))
	}

	if cmd.HasParent() {
		p := cmd.Parent()
		sections = append(sections, section(fmt.Sprintf("### See also\n\n* [%s](%s)\n", p.CommandPath(), cmdManualPath(p))))
	}

	var parts []string
	for _, s := range sections {
		if s != "" {
			parts = append(parts, string(s))
		}
	}
	_, err := io.WriteString(w, strings.Join(parts, "\n"))
	return err
}

// GenMarkdownTree generates one Markdown page per visible command of the tree
// into dir. A hidden command gets a page only with the markdown:generate
// annotation.
func GenMarkdownTree(cmd *cobra.Command, dir string) error {
	for _, c := range cmd.Commands() {
		if !visible(c) {
			continue
		}

		if err := GenMarkdownTree(c, dir); err != nil {
			return err
		}
	}

	f, err := os.Create(filepath.Join(dir, cmdManualPath(cmd)))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return genMarkdown(cmd, f)
}

func cmdManualPath(c *cobra.Command) string {
	if basenameOverride, found := c.Annotations["markdown:basename"]; found {
		return basenameOverride + ".md"
	}
	return strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"
}
