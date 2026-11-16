package collaborator

import (
	"github.com/spf13/cobra"
	addCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/collaborator/add"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/collaborator/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdCollaborator creates the `copia-cli repo collaborator` command group.
func NewCmdCollaborator(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collaborator <command>",
		Short: "Manage repository collaborators",
		Long:  "Manage the collaborators of a Copia repository.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		addCmd.NewCmdAdd(f),
	)

	return cmd
}
