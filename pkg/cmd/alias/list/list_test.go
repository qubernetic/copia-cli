package list

import (
	"errors"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	tests := []struct {
		name       string
		aliases    map[string]string
		wantOut    string
		wantStderr string
	}{
		{
			name: "aliases sorted by name",
			aliases: map[string]string{
				"il":    "issue list -L 1",
				"co":    "pr checkout",
				"igrep": `!copia-cli issue list --label "$1" | grep "$2"`,
			},
			wantOut: "co: pr checkout\nigrep: '!copia-cli issue list --label \"$1\" | grep \"$2\"'\nil: issue list -L 1\n",
		},
		{name: "no aliases", wantStderr: "no aliases configured\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			cfg := &config.Config{
				Hosts:   map[string]*config.HostConfig{"app.copia.io": {Token: "secret"}},
				Aliases: tt.aliases,
			}
			cmd := NewCmdList(&cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) { return cfg, nil }})
			cmd.SetArgs([]string{})

			require.NoError(t, cmd.Execute())
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.NotContains(t, stdout.String(), "secret")
		})
	}
}

func TestList_ConfigError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdList(&cmdutil.Factory{IOStreams: ios, Config: func() (*config.Config, error) { return nil, errors.New("parsing config: bad") }})
	cmd.SetArgs([]string{})
	cmd.SilenceUsage = true

	assert.EqualError(t, cmd.Execute(), "parsing config: bad")
}
