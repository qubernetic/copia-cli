package set

import (
	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// SetOptions holds all inputs for the config set command.
type SetOptions struct {
	IO         *iostreams.IOStreams
	ConfigPath string
	Key        string
	Value      string
}

// NewCmdSet creates the `copia-cli config set` command.
func NewCmdSet(f *cmdutil.Factory) *cobra.Command {
	opts := &SetOptions{}

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Update configuration with a value for the given key",
		Long:  "Validate a value and save it under a configuration key. An unknown key or a value the key does not allow fails without changing the config file; the hosts and their tokens are kept as they are.",
		Example: `  $ copia-cli config set output json
  $ copia-cli config set browser "firefox --new-tab"
  $ copia-cli config set editor vim`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO = f.IOStreams
			opts.ConfigPath = config.DefaultPath()
			opts.Key = args[0]
			opts.Value = args[1]
			return setRun(opts)
		},
	}

	return cmd
}

func setRun(opts *SetOptions) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	if err := cfg.Set(opts.Key, opts.Value); err != nil {
		return err
	}
	return config.Save(opts.ConfigPath, cfg)
}
