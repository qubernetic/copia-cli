package pr

import (
	"github.com/spf13/cobra"
	checkoutCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/checkout"
	closeCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/close"
	commentCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/comment"
	createCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/create"
	diffCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/diff"
	editCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/edit"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/list"
	mergeCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/merge"
	readyCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/ready"
	reopenCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/reopen"
	reviewCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/review"
	updateBranchCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/update-branch"
	viewCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/view"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdPR creates the `copia pr` command group.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Manage pull requests",
		Long:  "Work with Copia pull requests.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		createCmd.NewCmdCreate(f),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		viewCmd.NewCmdView(f),
		mergeCmd.NewCmdMerge(f),
		closeCmd.NewCmdClose(f),
		reopenCmd.NewCmdReopen(f),
		commentCmd.NewCmdComment(f),
		editCmd.NewCmdEdit(f),
		readyCmd.NewCmdReady(f),
		updateBranchCmd.NewCmdUpdateBranch(f),
		reviewCmd.NewCmdReview(f),
		diffCmd.NewCmdDiff(f),
		checkoutCmd.NewCmdCheckout(f),
	)

	return cmd
}
