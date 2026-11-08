// Package copiacmd provides the root command wiring and Main() entrypoint for the CLI.
package copiacmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/build"
	"github.com/qubernetic/copia-cli/internal/config"
	apiCmd "github.com/qubernetic/copia-cli/pkg/cmd/api"
	aliasCmd "github.com/qubernetic/copia-cli/pkg/cmd/alias"
	"github.com/qubernetic/copia-cli/pkg/cmd/alias/expand"
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
	sshkeyCmd "github.com/qubernetic/copia-cli/pkg/cmd/sshkey"
	repoCmd "github.com/qubernetic/copia-cli/pkg/cmd/repo"
	statusCmd "github.com/qubernetic/copia-cli/pkg/cmd/status"
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
	cmd.AddCommand(sshkeyCmd.NewCmdSSHKey(f))
	cmd.AddCommand(browseCmd.NewCmdBrowse(f))
	cmd.AddCommand(statusCmd.NewCmdStatus(f))
	cmd.AddCommand(configCmd.NewCmdConfig(f))
	cmd.AddCommand(aliasCmd.NewCmdAlias(f))
	cmd.AddCommand(completionCmd.NewCmdCompletion(f.IOStreams))

	return cmd
}

// NewFactory returns the Factory of a real run on ios: the config file at
// config.DefaultPath, the OS keyring, the repository of the git remote and
// the configured browser.
func NewFactory(ios *iostreams.IOStreams) *cmdutil.Factory {
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (*config.Config, error) {
			return config.Load(config.DefaultPath())
		},
		BaseRepo: cmdutil.DetectBaseRepo(),
		Keyring:  config.NewKeyringStore(),
	}
	f.Browser = cmdutil.NewBrowser(f.ConfigValue)
	return f
}

// Execute runs the root command of f with args. The command context carries
// f.OutputFormat, the default output format of the commands that take
// --json, and f.IOStreams pages through f.Pager.
//
// When args do not name a built-in command, an alias of the config file in
// args[0] is expanded first, so a built-in command always wins over an alias
// of the same name. A shell alias runs through sh instead of the root
// command; its non-zero exit status comes back as a *ShellAliasError.
func Execute(f *cmdutil.Factory, args []string) error {
	f.IOStreams.SetPager(f.Pager())
	rootCmd := NewRootCmd(f)
	rootCmd.InitDefaultHelpCmd()
	if cmd, _, err := rootCmd.Traverse(args); err != nil || cmd == rootCmd {
		expanded, isShell, err := expandAlias(f, args)
		if err != nil {
			return err
		}
		if isShell {
			return runShellAlias(f.IOStreams, expanded)
		}
		args = expanded
	}
	rootCmd.SetArgs(args)
	return rootCmd.ExecuteContext(cmdutil.WithOutputFormat(context.Background(), f.OutputFormat))
}

// expandAlias expands the alias in args[0], leaving a flag and cobra's
// completion requests alone without reading the config file.
func expandAlias(f *cmdutil.Factory, args []string) ([]string, bool, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") ||
		args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd {
		return args, false, nil
	}
	cfg, err := f.Config()
	if err != nil {
		return nil, false, err
	}
	return expand.ExpandAlias(cfg.Aliases, args)
}

// ShellAliasError is the non-zero exit status of a shell alias, which Main
// returns as the exit code of copia-cli without a message.
type ShellAliasError struct {
	Err *exec.ExitError
}

func (e *ShellAliasError) Error() string { return "shell alias: " + e.Err.Error() }

func (e *ShellAliasError) Unwrap() error { return e.Err }

// runShellAlias runs sh with args and the standard streams of ios.
func runShellAlias(ios *iostreams.IOStreams, args []string) error {
	sh, err := exec.LookPath("sh")
	if err != nil {
		return fmt.Errorf("running a shell alias: %w", err)
	}
	cmd := exec.Command(sh, args...)
	cmd.Stdin = ios.In
	cmd.Stdout = ios.Out
	cmd.Stderr = ios.ErrOut
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ShellAliasError{Err: exitErr}
	}
	if err != nil {
		return fmt.Errorf("running a shell alias: %w", err)
	}
	return nil
}

// Main is the entrypoint called from cmd/copia-cli/main.go.
func Main() int {
	// Note: env vars (COPIA_TOKEN, COPIA_HOST) are resolved in
	// Factory.ResolveAuth() with correct precedence:
	// flag > env var > config file
	f := NewFactory(iostreams.System())

	if err := Execute(f, os.Args[1:]); err != nil {
		var aliasErr *ShellAliasError
		if errors.As(err, &aliasErr) && aliasErr.Err.ExitCode() > 0 {
			return aliasErr.Err.ExitCode()
		}
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
