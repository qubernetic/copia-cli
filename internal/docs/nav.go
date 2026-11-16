package docs

import (
	"encoding/json"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// visible reports whether a command gets a page. A hidden command is left out
// unless it carries the markdown:generate annotation.
func visible(c *cobra.Command) bool {
	_, forced := c.Annotations["markdown:generate"]
	return !c.Hidden || forced
}

func visibleChildren(c *cobra.Command) []*cobra.Command {
	var res []*cobra.Command
	for _, child := range c.Commands() {
		if visible(child) {
			res = append(res, child)
		}
	}
	slices.SortFunc(res, func(a, b *cobra.Command) int { return strings.Compare(a.Name(), b.Name()) })
	return res
}

// CommandsNav returns the navigation of the command reference in the shape
// MkDocs uses for its nav setting. prefix is the path of the pages inside the
// docs directory, for example "manual/".
//
// The root's page is "Overview". Commands without visible subcommands come
// first, then the commands that have some, each as a group that opens with
// the command's own page. Both runs are sorted by name.
func CommandsNav(root *cobra.Command, prefix string) []any {
	res := []any{map[string]any{"Overview": prefix + cmdManualPath(root)}}
	return append(res, childrenNav(root, prefix)...)
}

func childrenNav(parent *cobra.Command, prefix string) []any {
	var leaves, groups []any
	for _, c := range visibleChildren(parent) {
		title := strings.TrimPrefix(c.CommandPath(), parent.Root().Name()+" ")
		page := map[string]any{title: prefix + cmdManualPath(c)}
		if len(visibleChildren(c)) == 0 {
			leaves = append(leaves, page)
			continue
		}
		group := append([]any{page}, childrenNav(c, prefix)...)
		groups = append(groups, map[string]any{title: group})
	}
	return append(leaves, groups...)
}

// WriteNav writes CommandsNav to path as indented JSON.
func WriteNav(root *cobra.Command, path, prefix string) error {
	data, err := json.MarshalIndent(CommandsNav(root, prefix), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
