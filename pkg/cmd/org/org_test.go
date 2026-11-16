package org

import (
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdOrg_Subcommands(t *testing.T) {
	cmd := NewCmdOrg(&cmdutil.Factory{})

	assert.Equal(t, "org <command>", cmd.Use)
	groups := map[string]string{}
	for _, c := range cmd.Commands() {
		groups[c.Name()] = c.GroupID
	}
	assert.Equal(t, map[string]string{
		"list": "General commands",
		"view": "Targeted commands",
	}, groups)

	var titles []string
	for _, g := range cmd.Groups() {
		titles = append(titles, g.Title)
	}
	assert.Equal(t, []string{"General commands", "Targeted commands"}, titles)
}
