package gpgkey

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdGPGKey_Subcommands(t *testing.T) {
	cmd := NewCmdGPGKey(&cmdutil.Factory{})

	assert.Equal(t, "gpg-key <command>", cmd.Use)
	assert.Contains(t, cmd.Long, "Copia account")

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list": "General commands",
		"add":  "General commands",
	}, groups)
}
