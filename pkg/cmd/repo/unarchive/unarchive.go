package unarchive

import (
	"github.com/qubernetic/copia-cli/pkg/cmd/repo/archive"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdUnarchive creates the `copia repo unarchive` command.
func NewCmdUnarchive(f *cmdutil.Factory) *cobra.Command {
	return archive.NewCmd(f, false, &cobra.Command{
		Use:   "unarchive [<owner/repo>]",
		Short: "Unarchive a repository",
		Long: `Unarchive a Copia repository.

With no argument, unarchives the current repository. There is no prompt, so
--yes is required.`,
		Example: `  # Unarchive the current repository
  $ copia-cli repo unarchive --yes

  # Unarchive a specific repository
  $ copia-cli repo unarchive my-org/my-repo --yes`,
	})
}
