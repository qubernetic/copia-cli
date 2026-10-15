package search

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdSearch_Subcommands(t *testing.T) {
	cmd := NewCmdSearch(&cmdutil.Factory{})
	assert.Equal(t, "search", cmd.Name())

	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
		assert.Equal(t, "General commands", c.GroupID)
	}
	assert.ElementsMatch(t, []string{"repos", "issues"}, names)

	groups := cmd.Groups()
	require.Len(t, groups, 1)
	assert.Equal(t, "General commands", groups[0].Title)
}
