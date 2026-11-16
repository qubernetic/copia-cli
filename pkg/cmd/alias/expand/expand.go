// Package expand turns the arguments of a copia-cli run that start with an
// alias name into the arguments of its expansion.
package expand

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

var placeholderRE = regexp.MustCompile(`\$\d+`)

// ExpandAlias returns args with the alias named by args[0] replaced by its
// expansion, or args unchanged when args[0] is no alias. Adapted from
// github.com/cli/cli/v2/pkg/cmd/root.ExpandAlias.
//
// An expansion that starts with "!" is a shell alias: isShell is true and
// expanded holds the arguments of sh, "-c" and the rest of the expansion,
// then "--" and the remaining arguments when there are any, so the script
// reads them as $1, $2 and so on.
//
// Any other expansion is split into words like a POSIX shell does, without
// expanding anything. A placeholder $1, $2 and so on in a word is replaced
// by that argument, which is never split again; without placeholders the
// remaining arguments are appended.
func ExpandAlias(aliases map[string]string, args []string) (expanded []string, isShell bool, err error) {
	if len(args) == 0 {
		return args, false, nil
	}
	expansion, ok := aliases[args[0]]
	if !ok {
		return args, false, nil
	}
	extra := args[1:]

	if script, ok := strings.CutPrefix(expansion, "!"); ok {
		expanded = []string{"-c", script}
		if len(extra) > 0 {
			expanded = append(expanded, "--")
			expanded = append(expanded, extra...)
		}
		return expanded, true, nil
	}

	words, err := cmdutil.SplitCommand(expansion)
	if err != nil {
		return nil, false, fmt.Errorf("alias %s: %w", args[0], err)
	}
	if !placeholderRE.MatchString(expansion) {
		return append(words, extra...), false, nil
	}

	for i, word := range words {
		var missing bool
		words[i] = placeholderRE.ReplaceAllStringFunc(word, func(p string) string {
			n, _ := strconv.Atoi(p[1:]) // the pattern matches digits only
			if n < 1 || n > len(extra) {
				missing = true
				return p
			}
			return extra[n-1]
		})
		if missing {
			return nil, false, fmt.Errorf("not enough arguments for alias: %s", expansion)
		}
	}
	return words, false, nil
}
