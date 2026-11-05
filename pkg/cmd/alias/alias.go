// Package alias provides the `copia-cli alias` command group.
package alias

import (
	"github.com/spf13/cobra"
	deleteCmd "github.com/qubernetic/copia-cli/pkg/cmd/alias/delete"
	importCmd "github.com/qubernetic/copia-cli/pkg/cmd/alias/imports"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/alias/list"
	setCmd "github.com/qubernetic/copia-cli/pkg/cmd/alias/set"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdAlias creates the `copia-cli alias` command group.
func NewCmdAlias(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias <command>",
		Short: "Create command shortcuts",
		Long: `Aliases can be used to make shortcuts for copia-cli commands or to compose multiple commands.

Run "copia-cli help alias set" to learn more.`,
	}

	cmdutil.AddGroup(cmd, "General commands",
		deleteCmd.NewCmdDelete(f),
		importCmd.NewCmdImport(f),
		listCmd.NewCmdList(f),
		setCmd.NewCmdSet(f),
	)

	return cmd
}
