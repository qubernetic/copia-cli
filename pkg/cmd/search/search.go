package search

import (
	"github.com/spf13/cobra"
	issuesCmd "github.com/qubernetic/copia-cli/pkg/cmd/search/issues"
	prsCmd "github.com/qubernetic/copia-cli/pkg/cmd/search/prs"
	reposCmd "github.com/qubernetic/copia-cli/pkg/cmd/search/repos"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

func NewCmdSearch(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <command>",
		Short: "Search across Copia",
		Long:  "Search repositories, issues and pull requests.",
	}

	cmdutil.AddGroup(cmd, "General commands",
		reposCmd.NewCmdSearchRepos(f),
		issuesCmd.NewCmdSearchIssues(f),
		prsCmd.NewCmdSearchPRs(f),
	)

	return cmd
}
