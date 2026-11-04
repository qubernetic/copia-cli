package pr

import (
	"github.com/spf13/cobra"
	checkoutCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/checkout"
	closeCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/close"
	createCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/create"
	diffCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/diff"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/list"
	mergeCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/merge"
	reopenCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/reopen"
	reviewCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr/review"
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
		reviewCmd.NewCmdReview(f),
		diffCmd.NewCmdDiff(f),
		checkoutCmd.NewCmdCheckout(f),
	)

	return cmd
}
