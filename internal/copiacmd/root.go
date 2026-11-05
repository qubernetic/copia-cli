// Package copiacmd provides the root command wiring and Main() entrypoint for the CLI.
package copiacmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/build"
	"github.com/qubernetic/copia-cli/internal/config"
	apiCmd "github.com/qubernetic/copia-cli/pkg/cmd/api"
	authCmd "github.com/qubernetic/copia-cli/pkg/cmd/auth"
	browseCmd "github.com/qubernetic/copia-cli/pkg/cmd/browse"
	completionCmd "github.com/qubernetic/copia-cli/pkg/cmd/completion"
	configCmd "github.com/qubernetic/copia-cli/pkg/cmd/config"
	issueCmd "github.com/qubernetic/copia-cli/pkg/cmd/issue"
	labelCmd "github.com/qubernetic/copia-cli/pkg/cmd/label"
	notificationCmd "github.com/qubernetic/copia-cli/pkg/cmd/notification"
	orgCmd "github.com/qubernetic/copia-cli/pkg/cmd/org"
	prCmd "github.com/qubernetic/copia-cli/pkg/cmd/pr"
	releaseCmd "github.com/qubernetic/copia-cli/pkg/cmd/release"
	searchCmd "github.com/qubernetic/copia-cli/pkg/cmd/search"
	repoCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// NewRootCmd creates the root `copia-cli` command with all subcommands.
func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "copia-cli <command> <subcommand> [flags]",
		Short:         "Copia CLI — source control for industrial automation",
		Long:          "Work with Copia repositories, issues, pull requests, and more from the command line.",
		Version:       build.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetVersionTemplate(build.VersionInfo() + "\n")

	// Global flags — bound to factory fields, resolved in ResolveAuth()
	cmd.PersistentFlags().StringVar(&f.Host, "host", "", "Target Copia host")
	cmd.PersistentFlags().StringVar(&f.Token, "token", "", "Authentication token")
	cmd.PersistentFlags().StringVarP(&f.Repo, "repo", "R", "", "Select repository (owner/repo)")

	cmd.AddCommand(authCmd.NewCmdAuth(f))
	cmd.AddCommand(repoCmd.NewCmdRepo(f))
	cmd.AddCommand(issueCmd.NewCmdIssue(f))
	cmd.AddCommand(labelCmd.NewCmdLabel(f))
	cmd.AddCommand(prCmd.NewCmdPR(f))
	cmd.AddCommand(releaseCmd.NewCmdRelease(f))
	cmd.AddCommand(apiCmd.NewCmdAPI(f))
	cmd.AddCommand(searchCmd.NewCmdSearch(f))
	cmd.AddCommand(orgCmd.NewCmdOrg(f))
	cmd.AddCommand(notificationCmd.NewCmdNotification(f))
	cmd.AddCommand(browseCmd.NewCmdBrowse(f))
	cmd.AddCommand(configCmd.NewCmdConfig(f))
	cmd.AddCommand(completionCmd.NewCmdCompletion(f.IOStreams))

	return cmd
}

// NewFactory returns the Factory of a real run on ios: the config file at
// config.DefaultPath, the repository of the git remote and the configured
// browser.
func NewFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return config.Load(config.DefaultPath())
		},
		BaseRepo: cmdutil.DetectBaseRepo(),
	}
	f.Browser = cmdutil.NewBrowser(f.ConfigValue)
	return f
}

// Execute runs the root command of f with args. The command context carries
// f.OutputFormat, the default output format of the commands that take
// --json.
func Execute(f *cmdutil.Factory, args []string) error {
	rootCmd := NewRootCmd(f)
	rootCmd.SetArgs(args)
	return rootCmd.ExecuteContext(cmdutil.WithOutputFormat(context.Background(), f.OutputFormat))
}

// Main is the entrypoint called from cmd/copia-cli/main.go.
func Main() int {
	// Note: env vars (COPIA_TOKEN, COPIA_HOST) are resolved in
	// Factory.ResolveAuth() with correct precedence:
	// flag > env var > config file
	f := NewFactory(iostreams.System())

	if err := Execute(f, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
