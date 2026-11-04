package issue

import (
	"github.com/spf13/cobra"
	closeCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/close"
	commentCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/comment"
	createCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/create"
	deleteCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/delete"
	editCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/edit"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/list"
	reopenCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/reopen"
	viewCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue/view"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdIssue creates the `copia issue` command group.
func NewCmdIssue(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "Manage issues",
		Long:  "Work with Copia repository issues.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		createCmd.NewCmdCreate(f),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		viewCmd.NewCmdView(f),
		closeCmd.NewCmdClose(f),
		reopenCmd.NewCmdReopen(f),
		commentCmd.NewCmdComment(f),
		editCmd.NewCmdEdit(f),
		deleteCmd.NewCmdDelete(f),
	)

	return cmd
}
