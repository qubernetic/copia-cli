package label

import (
	"github.com/spf13/cobra"
	createCmd "github.com/qubernetic/copia-cli/pkg/cmd/label/create"
	deleteCmd "github.com/qubernetic/copia-cli/pkg/cmd/label/delete"
	editCmd "github.com/qubernetic/copia-cli/pkg/cmd/label/edit"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/label/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdLabel creates the `copia label` command group.
func NewCmdLabel(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label <command>",
		Short: "Manage labels",
		Long:  "Work with repository labels.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		createCmd.NewCmdCreate(f),
		editCmd.NewCmdEdit(f),
		deleteCmd.NewCmdDelete(f),
	)

	return cmd
}
