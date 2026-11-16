// Package imports provides `copia-cli alias import`; the name avoids the Go
// keyword, as in gh.
package imports

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmd/alias/shared"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ImportOptions holds all inputs for the alias import command.
type ImportOptions struct {
	IO                *iostreams.IOStreams
	ConfigPath        string
	Root              *cobra.Command
	Filename          string
	OverwriteExisting bool
}

// NewCmdImport creates the `copia-cli alias import` command.
func NewCmdImport(f *cmdutil.Factory) *cobra.Command {
	opts := &ImportOptions{}

	cmd := &cobra.Command{
		Use:   "import [<filename> | -]",
		Short: "Import aliases from a YAML file",
		Long: `Import aliases from the contents of a YAML file.

Aliases should be defined as a map in YAML, where the keys represent aliases
and the values represent the corresponding expansions. An example file should
look like the following:

    bugs: issue list --label=bug
    igrep: '!copia-cli issue list --label="$1" | grep "$2"'
    co: pr checkout

Use "-" to read aliases (in YAML format) from standard input; without an
argument they are read from standard input too, unless it is a terminal.

The output of "copia-cli alias list" is such a file. Each alias is checked
like "copia-cli alias set" checks it: an alias that is refused is reported on
standard error, the others are imported, and the command fails at the end.
An alias that already exists is kept unless "--clobber" is given.`,
		Example: `  # Import aliases from a file
  $ copia-cli alias import aliases.yml

  # Import aliases from standard input
  $ copia-cli alias import -`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if f.IOStreams.IsStdinTTY() {
					return errors.New("no filename passed and nothing on standard input")
				}
				opts.Filename = "-"
			} else {
				opts.Filename = args[0]
			}
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.Root = cmd.Root()
			return importRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.OverwriteExisting, "clobber", false, "Overwrite existing aliases of the same name")

	return cmd
}

func importRun(opts *ImportOptions) error {
	data, err := cmdutil.ReadFile(opts.Filename, opts.IO.In)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", opts.Filename, err)
	}
	var aliases map[string]string
	if err := yaml.Unmarshal(data, &aliases); err != nil {
		return fmt.Errorf("could not parse %s: %w", opts.Filename, err)
	}
	if len(aliases) == 0 {
		return nil
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}

	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	slices.Sort(names)

	var imported []string
	failed := 0
	for _, name := range names {
		expansion := aliases[name]
		if err := validate(opts, cfg, name, expansion); err != nil {
			_, _ = fmt.Fprintf(opts.IO.ErrOut, "could not import alias %s: %v\n", name, err)
			failed++
			continue
		}
		verb := "Added"
		if _, exists := cfg.Aliases[name]; exists {
			verb = "Changed"
		}
		cfg.Aliases[name] = expansion
		imported = append(imported, verb+" alias "+name)
	}

	if len(imported) > 0 {
		if err := config.Save(opts.ConfigPath, cfg); err != nil {
			return err
		}
	}
	for _, line := range imported {
		_, _ = fmt.Fprintln(opts.IO.Out, line)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d aliases not imported", failed, len(names))
	}
	return nil
}

func validate(opts *ImportOptions, cfg *config.Config, name, expansion string) error {
	if err := shared.ValidateName(opts.Root, name); err != nil {
		return err
	}
	if err := shared.ValidateExpansion(opts.Root, expansion); err != nil {
		return err
	}
	if _, exists := cfg.Aliases[name]; exists && !opts.OverwriteExisting {
		return errors.New("name already taken; pass --clobber to overwrite it")
	}
	return nil
}
