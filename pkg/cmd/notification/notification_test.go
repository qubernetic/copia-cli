package notification

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdNotification_Subcommands(t *testing.T) {
	cmd := NewCmdNotification(&cmdutil.Factory{})
	assert.Equal(t, "notification", cmd.Name())

	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
		assert.Equal(t, "General commands", c.GroupID)
	}
	assert.ElementsMatch(t, []string{"list", "read"}, names)

	groups := cmd.Groups()
	require.Len(t, groups, 1)
	assert.Equal(t, "General commands", groups[0].Title)
}
