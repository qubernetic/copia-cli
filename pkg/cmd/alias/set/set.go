package set

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmd/alias/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// SetOptions holds all inputs for the alias set command.
type SetOptions struct {
	IO                *iostreams.IOStreams
	ConfigPath        string
	Root              *cobra.Command
	Name              string
	Expansion         string
	IsShell           bool
	OverwriteExisting bool
}

// NewCmdSet creates the `copia-cli alias set` command.
func NewCmdSet(f *cmdutil.Factory) *cobra.Command {
	opts := &SetOptions{}

	cmd := &cobra.Command{
		Use:   "set <alias> <expansion>",
		Short: "Create a shortcut for a copia-cli command",
		Long: `Define a word that will expand to a full copia-cli command when invoked.

The expansion may specify additional arguments and flags. If the expansion
includes positional placeholders such as "$1", extra arguments that follow
the alias are inserted appropriately. Otherwise, extra arguments are appended
to the expanded command. An argument fills a placeholder as one word and is
never split again.

Use "-" as the expansion argument to read the expansion string from standard
input.

If the expansion starts with "!" or if "--shell" was given, the expansion is
a shell expression evaluated through the "sh" interpreter; extra arguments
are passed to it as "$1", "$2" and so on. This allows compounding commands
with "|" to pipe output to other commands.

An alias may not have the name of a copia-cli command, and the expansion of
an alias that is no shell expression must start with a copia-cli command.
Aliases are saved under "aliases" in the config file.

Quotes must always be used when defining a command as in the examples.`,
		Example: `  $ copia-cli alias set il 'issue list -L 1'
  $ copia-cli il
  #=> copia-cli issue list -L 1

  $ copia-cli alias set co 'pr checkout'
  $ copia-cli co 23
  #=> copia-cli pr checkout 23

  $ copia-cli alias set bugs 'issue list --label $1'
  $ copia-cli bugs urgent
  #=> copia-cli issue list --label urgent

  $ copia-cli alias set --shell igrep 'copia-cli issue list --label "$1" | grep "$2"'
  $ copia-cli igrep epic foo
  #=> copia-cli issue list --label epic | grep foo`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.Root = cmd.Root()
			opts.Name = args[0]
			opts.Expansion = args[1]
			return setRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.IsShell, "shell", "s", false, "Declare an alias to be passed through a shell interpreter")
	cmd.Flags().BoolVar(&opts.OverwriteExisting, "clobber", false, "Overwrite existing aliases of the same name")

	return cmd
}

func setRun(opts *SetOptions) error {
	expansion := opts.Expansion
	if expansion == "-" {
		data, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return fmt.Errorf("failed to read expansion from standard input: %w", err)
		}
		expansion = strings.TrimRight(string(data), "\r\n")
	}
	if opts.IsShell && !strings.HasPrefix(expansion, "!") {
		expansion = "!" + expansion
	}

	if err := shared.ValidateName(opts.Root, opts.Name); err != nil {
		return fmt.Errorf("could not create alias %s: %w", opts.Name, err)
	}
	if err := shared.ValidateExpansion(opts.Root, expansion); err != nil {
		return fmt.Errorf("could not create alias %s: %w", opts.Name, err)
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	_, exists := cfg.Aliases[opts.Name]
	if exists && !opts.OverwriteExisting {
		return fmt.Errorf("could not create alias %s: name already taken; pass --clobber to overwrite it", opts.Name)
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	cfg.Aliases[opts.Name] = expansion
	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}

	verb := "Added"
	if exists {
		verb = "Changed"
	}
	_, _ = fmt.Fprintf(opts.IO.Out, "%s alias %s\n", verb, opts.Name)
	return nil
}
