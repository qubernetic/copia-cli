package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ListOptions holds all inputs for the config list command.
type ListOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

// NewCmdList creates the `copia-cli config list` command.
func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Print a list of configuration keys and values",
		Long:    "Print every configuration key as key=value, with the default of a key that is not set. Host credentials are not printed.",
		Example: "  $ copia-cli config list",
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
	for _, o := range config.Options {
		value, _ := cfg.Get(o.Key) // every key of config.Options is known
		_, _ = fmt.Fprintf(opts.IO.Out, "%s=%s\n", o.Key, value)
	}
	return nil
}
