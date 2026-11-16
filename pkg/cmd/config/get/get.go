package get

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// GetOptions holds all inputs for the config get command.
type GetOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Key    string
}

// NewCmdGet creates the `copia-cli config get` command.
func NewCmdGet(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{}

	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the value of a given configuration key",
		Long:  "Print the value of a configuration key, or its default when the key is not set. A key without a value and without a default prints nothing.",
		Example: `  $ copia-cli config get output
  table`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.Config = f.Config
			opts.Key = args[0]
			return getRun(opts)
		},
	}

	return cmd
}

func getRun(opts *GetOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	value, err := cfg.Get(opts.Key)
	if err != nil {
		return err
	}
	if value != "" {
		_, _ = fmt.Fprintln(opts.IO.Out, value)
	}
	return nil
}
