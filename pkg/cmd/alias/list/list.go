package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ListOptions holds all inputs for the alias list command.
type ListOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

// NewCmdList creates the `copia-cli alias list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your aliases",
		Long:    "Print the aliases of the config file as a YAML map of name to expansion, sorted by name, the format that `copia-cli alias import` reads.",
		Example: "  $ copia-cli alias list > aliases.yml",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Config = f.Config
			return listRun(opts)
		},
	}

	return cmd
}

func listRun(opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if len(cfg.Aliases) == 0 {
		_, _ = fmt.Fprintln(opts.IO.ErrOut, "no aliases configured")
		return nil
	}
	out, _ := yaml.Marshal(cfg.Aliases) // a map of strings always marshals
	_, err = opts.IO.Out.Write(out)
	return err
}
