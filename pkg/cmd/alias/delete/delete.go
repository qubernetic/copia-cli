package delete

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// DeleteOptions holds all inputs for the alias delete command.
type DeleteOptions struct {
	IO         *iostreams.IOStreams
	ConfigPath string
	Name       string
	All        bool
}

// NewCmdDelete creates the `copia-cli alias delete` command.
func NewCmdDelete(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{}

	cmd := &cobra.Command{
		Use:   "delete {<alias> | --all}",
		Short: "Delete set aliases",
		Long:  "Delete one alias, or every alias with --all, from the config file. The hosts and their tokens are kept as they are.",
		Example: `  $ copia-cli alias delete co
  $ copia-cli alias delete --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !opts.All {
				return errors.New("must specify an alias to delete or --all")
			}
			if len(args) > 0 && opts.All {
				return errors.New("cannot use --all with an alias name")
			}
			if len(args) > 0 {
				opts.Name = args[0]
			}
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.All, "all", false, "Delete all aliases")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	var names []string
	if opts.All {
		if len(cfg.Aliases) == 0 {
			return errors.New("no aliases configured")
		}
		for name := range cfg.Aliases {
			names = append(names, name)
		}
		slices.Sort(names)
	} else {
		if _, ok := cfg.Aliases[opts.Name]; !ok {
			return fmt.Errorf("no such alias %s", opts.Name)
		}
		names = []string{opts.Name}
	}

	expansions := make([]string, len(names))
	for i, name := range names {
		expansions[i] = cfg.Aliases[name]
		delete(cfg.Aliases, name)
	}
	if err := config.Save(opts.ConfigPath, cfg); err != nil {
		return err
	}
	for i, name := range names {
		_, _ = fmt.Fprintf(opts.IO.Out, "Deleted alias %s; was %s\n", name, expansions[i])
	}
	return nil
}
