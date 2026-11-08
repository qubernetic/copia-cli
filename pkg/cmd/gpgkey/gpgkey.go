package gpgkey

import (
	"github.com/spf13/cobra"
	addCmd "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/add"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/gpgkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdGPGKey creates the `copia-cli gpg-key` command group.
func NewCmdGPGKey(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gpg-key <command>",
		Short: "Manage GPG keys",
		Long:  "Manage GPG keys registered with your Copia account.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		addCmd.NewCmdAdd(f),
	)

	return cmd
}
