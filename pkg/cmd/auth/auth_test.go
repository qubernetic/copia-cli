package auth

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdAuth_Subcommands(t *testing.T) {
	cmd := NewCmdAuth(&cmdutil.Factory{})

	assert.Equal(t, "auth <command>", cmd.Use)
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	assert.ElementsMatch(t, []string{"login", "logout", "status"}, names)
}
