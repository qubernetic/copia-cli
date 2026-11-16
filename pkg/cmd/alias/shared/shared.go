// Package shared holds the checks that `copia-cli alias set` and `alias
// import` run on an alias before they save it.
package shared

import (
	"errors"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// The reasons an alias is refused. A caller names the alias and its action:
// "could not create alias co: already a copia-cli command".
var (
	ErrInvalidName      = errors.New("a name may not be empty, start with - or contain blanks")
	ErrCommandName      = errors.New("already a copia-cli command")
	ErrInvalidExpansion = errors.New("expansion does not correspond to a copia-cli command")
)

// ValidateName fails when name is empty, starts with "-", contains a blank
// or names a built-in command of root, which an alias may not shadow.
func ValidateName(root *cobra.Command, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return ErrInvalidName
	}
	if isCommand(root, []string{name}) {
		return ErrCommandName
	}
	return nil
}

// ValidateExpansion fails when expansion is no shell alias (it does not
// start with "!") and does not start with a built-in command of root.
func ValidateExpansion(root *cobra.Command, expansion string) error {
	if strings.HasPrefix(expansion, "!") {
		return nil
	}
	words, err := cmdutil.SplitCommand(expansion)
	if err != nil || !isCommand(root, words) {
		return ErrInvalidExpansion
	}
	return nil
}

func isCommand(root *cobra.Command, args []string) bool {
	cmd, _, err := root.Traverse(args)
	return err == nil && cmd != root
}
