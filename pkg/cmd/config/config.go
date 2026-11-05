// Package config provides the `copia-cli config` command group.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	getCmd "github.com/qubernetic/copia-cli/pkg/cmd/config/get"
	listCmd "github.com/qubernetic/copia-cli/pkg/cmd/config/list"
	setCmd "github.com/qubernetic/copia-cli/pkg/cmd/config/set"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
)

// NewCmdConfig creates the `copia-cli config` command group.
func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	var long strings.Builder
	long.WriteString("Display or change configuration settings for copia-cli.\n\nCurrent respected settings:\n\n")
	for _, o := range config.Options {
		_, _ = fmt.Fprintf(&long, "- `%s`: %s", o.Key, o.Description)
		if len(o.AllowedValues) > 0 {
			_, _ = fmt.Fprintf(&long, " `{%s}`", strings.Join(o.AllowedValues, " | "))
		}
		if o.DefaultValue != "" {
			_, _ = fmt.Fprintf(&long, " (default `%s`)", o.DefaultValue)
		}
		long.WriteString("\n")
	}

	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Manage configuration for copia-cli",
		Long:  long.String(),
	}

	cmdutil.AddGroup(cmd, "General commands",
		getCmd.NewCmdGet(f),
		listCmd.NewCmdList(f),
		setCmd.NewCmdSet(f),
	)

	return cmd
}
