package repo

import (
	"github.com/spf13/cobra"
	cloneCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/clone"
	createCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/create"
	deleteCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/delete"
	forkCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/fork"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/list"
	setDefaultCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/setdefault"
	viewCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo/view"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdRepo creates the `copia repo` command group.
func NewCmdRepo(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <command>",
		Short: "Manage repositories",
		Long:  "Work with Copia repositories.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		cloneCmd.NewCmdClone(f),
		createCmd.NewCmdCreate(f),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		viewCmd.NewCmdView(f),
		deleteCmd.NewCmdDelete(f),
		forkCmd.NewCmdFork(f),
		setDefaultCmd.NewCmdSetDefault(f),
	)

	return cmd
}
