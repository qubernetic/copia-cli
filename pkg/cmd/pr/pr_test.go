package pr

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdPR_RegistersSubcommands(t *testing.T) {
	cmd := NewCmdPR(&cmdutil.Factory{})

	assert.Equal(t, "pr <command>", cmd.Use)

	groupOf := map[string]string{}
	for _, c := range cmd.Commands() {
		groupOf[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list":     "General commands",
		"create":   "General commands",
		"view":     "Targeted commands",
		"merge":    "Targeted commands",
		"close":    "Targeted commands",
		"review":   "Targeted commands",
		"diff":     "Targeted commands",
		"checkout": "Targeted commands",
	}, groupOf)

	var titles []string
	for _, g := range cmd.Groups() {
		titles = append(titles, g.Title)
	}
	assert.Equal(t, []string{"General commands", "Targeted commands"}, titles)
}
