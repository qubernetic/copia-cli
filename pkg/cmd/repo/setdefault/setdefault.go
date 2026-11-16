package setdefault

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type SetDefaultOptions struct {
	IO        *iostreams.IOStreams
	Repo      string // owner/repo or a remote name, empty without an argument
	ViewMode  bool
	UnsetMode bool
}

func NewCmdSetDefault(f *cmdutil.Factory) *cobra.Command {
	opts := &SetDefaultOptions{}

	cmd := &cobra.Command{
		Use:   "set-default [<repository>]",
		Short: "Configure default repository for this directory",
		Long: `Set the default repository of the local git repository.

copia-cli uses the default repository for the commands that work on the
current repository, such as issue, pr, label and release. It is stored as
the git config key remote.<name>.copia-resolved on the remote that points to
it, and wins over the remote "origin". The -R flag still wins over both.

The repository is given as owner/repo or as the name of a git remote. Without
an argument the only remote is used; with several remotes the command fails
and lists them.`,
		Example: `  # Set a repository explicitly
  $ copia-cli repo set-default my-org/my-repo

  # Set a repository using a git remote name
  $ copia-cli repo set-default upstream

  # View the current default repository
  $ copia-cli repo set-default --view

  # Remove the default repository
  $ copia-cli repo set-default --unset`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			if len(args) > 0 {
				if opts.ViewMode || opts.UnsetMode {
					return fmt.Errorf("the repository argument cannot be used with --view or --unset")
				}
				opts.Repo = args[0]
			}
			return SetDefaultRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ViewMode, "view", "v", false, "View the current default repository")
	cmd.Flags().BoolVarP(&opts.UnsetMode, "unset", "u", false, "Unset the current default repository")
	cmd.MarkFlagsMutuallyExclusive("view", "unset")

	return cmd
}

func SetDefaultRun(opts *SetDefaultOptions) error {
	remotes, err := cmdutil.ListRemotes()
	if err != nil {
		return err
	}

	var current *cmdutil.Remote
	for i := range remotes {
		if remotes[i].Resolved {
			current = &remotes[i]
			break
		}
	}

	if opts.ViewMode {
		if current == nil {
			_, _ = fmt.Fprintln(opts.IO.ErrOut, "! No default remote repository has been set. To learn more about the default repository, run: copia-cli repo set-default --help")
			return nil
		}
		_, _ = fmt.Fprintln(opts.IO.Out, current.FullName())
		return nil
	}

	if opts.UnsetMode {
		// Also removes a mark on a remote that ListRemotes leaves out.
		if err := cmdutil.UnsetDefaultRemote(); err != nil {
			return err
		}
		if current == nil {
			_, _ = fmt.Fprintln(opts.IO.ErrOut, "! No default repository has been set")
			return nil
		}
		_, _ = fmt.Fprintf(opts.IO.Out, "Unset %s as default repository\n", current.FullName())
		return nil
	}

	if len(remotes) == 0 {
		return fmt.Errorf("none of the git remotes correspond to a valid remote repository")
	}

	selected, err := selectRemote(opts, remotes)
	if err != nil {
		return err
	}
	if err := cmdutil.SetDefaultRemote(selected.Name); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "Set %s as the default repository for the current directory\n", selected.FullName())
	return nil
}

// selectRemote returns the remote named by opts.Repo (owner/repo, any case,
// or a remote name), or the only remote when there is no argument.
func selectRemote(opts *SetDefaultOptions, remotes []cmdutil.Remote) (cmdutil.Remote, error) {
	if opts.Repo == "" {
		if len(remotes) == 1 {
			_, _ = fmt.Fprintf(opts.IO.Out, "Found only one known remote repo, %s.\n", remotes[0].FullName())
			return remotes[0], nil
		}
		width := 0
		for _, r := range remotes {
			width = max(width, len(r.Name))
		}
		var b strings.Builder
		b.WriteString("repository required when there are several git remotes:")
		for _, r := range remotes {
			fmt.Fprintf(&b, "\n  %-*s  %s", width, r.Name, r.FullName())
		}
		return cmdutil.Remote{}, fmt.Errorf("%s", b.String())
	}

	for _, r := range remotes {
		if strings.Contains(opts.Repo, "/") && strings.EqualFold(r.FullName(), opts.Repo) {
			return r, nil
		}
	}
	for _, r := range remotes {
		if r.Name == opts.Repo {
			return r, nil
		}
	}
	return cmdutil.Remote{}, fmt.Errorf("%s does not correspond to any git remotes", opts.Repo)
}
