package config

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdConfig_Subcommands(t *testing.T) {
	cmd := NewCmdConfig(&cmdutil.Factory{})

	assert.Equal(t, "config <command>", cmd.Use)

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"get":  "General commands",
		"list": "General commands",
	}, groups)
}

func TestNewCmdConfig_LongListsSettings(t *testing.T) {
	cmd := NewCmdConfig(&cmdutil.Factory{})

	assert.Contains(t, cmd.Long, "- `output`: the default output format of the commands that take --json `{table | json}` (default `table`)\n")
	assert.Contains(t, cmd.Long, "- `browser`: the web browser to use for opening URLs\n")
	assert.Contains(t, cmd.Long, "- `pager`: ")
	assert.Contains(t, cmd.Long, "- `editor`: ")
}
