package alias

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdAlias_Subcommands(t *testing.T) {
	cmd := NewCmdAlias(&cmdutil.Factory{})

	assert.Equal(t, "alias <command>", cmd.Use)

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"delete": "General commands",
		"list":   "General commands",
		"set":    "General commands",
	}, groups)
}
