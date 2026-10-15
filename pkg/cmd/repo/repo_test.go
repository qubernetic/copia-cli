package repo

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRepo_Subcommands(t *testing.T) {
	cmd := NewCmdRepo(&cmdutil.Factory{})

	assert.Equal(t, "repo <command>", cmd.Use)

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list":   "General commands",
		"clone":  "General commands",
		"create": "General commands",
		"view":   "Targeted commands",
		"delete": "Targeted commands",
		"fork":   "Targeted commands",
	}, groups)

	var titles []string
	for _, g := range cmd.Groups() {
		titles = append(titles, g.Title)
	}
	assert.Equal(t, []string{"General commands", "Targeted commands"}, titles)
}
