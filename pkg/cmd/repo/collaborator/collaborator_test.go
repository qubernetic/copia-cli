package collaborator

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdCollaborator_Subcommands(t *testing.T) {
	cmd := NewCmdCollaborator(&cmdutil.Factory{})

	assert.Equal(t, "collaborator <command>", cmd.Use)

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list": "General commands",
		"add":  "General commands",
	}, groups)
}
