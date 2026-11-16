package auth

import (
	"github.com/spf13/cobra"
	gitCredentialCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/gitcredential"
	loginCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/login"
	logoutCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/logout"
	setupGitCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/setupgit"
	statusCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/status"
	switchCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/switch"
	tokenCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth/token"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdAuth creates the `copia auth` command group.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate with Copia",
		Long:  "Manage authentication state for Copia instances.",
	}

	cmd.AddCommand(loginCmd.NewCmdLogin(f))
	cmd.AddCommand(logoutCmd.NewCmdLogout(f))
	cmd.AddCommand(statusCmd.NewCmdStatus(f))
	cmd.AddCommand(switchCmd.NewCmdSwitch(f))
	cmd.AddCommand(tokenCmd.NewCmdToken(f))
	cmd.AddCommand(gitCredentialCmd.NewCmdCredential(f))
	cmd.AddCommand(setupGitCmd.NewCmdSetupGit(f))

	return cmd
}
