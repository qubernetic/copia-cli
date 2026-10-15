package release

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdRelease_Subcommands(t *testing.T) {
	cmd := NewCmdRelease(&cmdutil.Factory{})

	assert.Equal(t, "release <command>", cmd.Use)

	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list":   "General commands",
		"create": "General commands",
		"delete": "Targeted commands",
		"upload": "Targeted commands",
	}, groups)

	var titles []string
	for _, g := range cmd.Groups() {
		titles = append(titles, g.Title)
	}
	require.Len(t, titles, 2)
	assert.Equal(t, []string{"General commands", "Targeted commands"}, titles)
}
