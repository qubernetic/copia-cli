package issue

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdIssue_RegistersSubcommands(t *testing.T) {
	cmd := NewCmdIssue(&cmdutil.Factory{})

	assert.Equal(t, "issue <command>", cmd.Use)

	groupOf := map[string]string{}
	for _, c := range cmd.Commands() {
		groupOf[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list":    "General commands",
		"create":  "General commands",
		"view":    "Targeted commands",
		"close":   "Targeted commands",
		"comment": "Targeted commands",
		"edit":    "Targeted commands",
	}, groupOf)

	var titles []string
	for _, g := range cmd.Groups() {
		titles = append(titles, g.Title)
	}
	require.Len(t, titles, 2)
	assert.Equal(t, []string{"General commands", "Targeted commands"}, titles)
}
