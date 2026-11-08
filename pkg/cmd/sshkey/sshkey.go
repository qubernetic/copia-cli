package sshkey

import (
	"github.com/spf13/cobra"
	addCmd "github.com/qubernetic/copia-cli/pkg/cmd/sshkey/add"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/sshkey/list"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdSSHKey creates the `copia-cli ssh-key` command group.
func NewCmdSSHKey(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-key <command>",
		Short: "Manage SSH keys",
		Long:  "Manage SSH keys registered with your Copia account.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		listCmd.NewCmdList(f),
		addCmd.NewCmdAdd(f),
	)

	return cmd
}
