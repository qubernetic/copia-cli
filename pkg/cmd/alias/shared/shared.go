// Package shared holds the checks that `copia-cli alias set` and `alias
// import` run on an alias before they save it.
package shared

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// ValidateName fails when name is empty, starts with "-", contains a blank
// or names a built-in command of root, which an alias may not shadow.
func ValidateName(root *cobra.Command, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return fmt.Errorf("could not create alias %q: a name may not be empty, start with - or contain blanks", name)
	}
	if isCommand(root, []string{name}) {
		return fmt.Errorf("could not create alias %s: already a copia-cli command", name)
	}
	return nil
}

// ValidateExpansion fails when expansion is no shell alias (it does not
// start with "!") and does not start with a built-in command of root.
func ValidateExpansion(root *cobra.Command, name, expansion string) error {
	if strings.HasPrefix(expansion, "!") {
		return nil
	}
	words, err := cmdutil.SplitCommand(expansion)
	if err != nil || !isCommand(root, words) {
		return fmt.Errorf("could not create alias %s: expansion does not correspond to a copia-cli command", name)
	}
	return nil
}

func isCommand(root *cobra.Command, args []string) bool {
	cmd, _, err := root.Traverse(args)
	return err == nil && cmd != root
}
