package label

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdLabel_Subcommands(t *testing.T) {
	cmd := NewCmdLabel(&cmdutil.Factory{})

	assert.Equal(t, "label <command>", cmd.Use)

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list":   "General commands",
		"create": "General commands",
	}, groups)

	var titles []string
	for _, g := range cmd.Groups() {
		titles = append(titles, g.Title)
	}
	assert.Equal(t, []string{"General commands"}, titles)
}
