package release

import (
	"github.com/spf13/cobra"
	createCmd "github.com/qubernetic/copia-cli/pkg/cmd/release/create"
	deleteCmd "github.com/qubernetic/copia-cli/pkg/cmd/release/delete"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/release/list"
	uploadCmd "github.com/qubernetic/copia-cli/pkg/cmd/release/upload"
	viewCmd "github.com/qubernetic/copia-cli/pkg/cmd/release/view"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func NewCmdRelease(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <command>",
		Short: "Manage releases",
		Long:  "Work with Copia repository releases.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		createCmd.NewCmdCreate(f),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		deleteCmd.NewCmdDelete(f),
		uploadCmd.NewCmdUpload(f),
		viewCmd.NewCmdView(f),
	)

	return cmd
}
