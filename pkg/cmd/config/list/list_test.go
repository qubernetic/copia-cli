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

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		cfg     *config.Config
		cfgErr  error
		wantOut string
		wantErr string
	}{
		{
			name:    "defaults",
			cfg:     &config.Config{},
			wantOut: "output=table\nbrowser=\npager=\neditor=\n",
		},
		{
			name: "set keys",
			cfg: &config.Config{
				Hosts:   map[string]*config.HostConfig{"app.copia.io": {Token: "secret", User: "john"}},
				Output:  "json",
				Browser: "firefox --new-tab",
				Pager:   "less -R",
				Editor:  "vim",
			},
			wantOut: "output=json\nbrowser=firefox --new-tab\npager=less -R\neditor=vim\n",
		},
		{
			name:    "unreadable config",
			cfgErr:  errors.New("parsing config: bad yaml"),
			wantErr: "parsing config: bad yaml",
		},
		{
			name:    "extra argument",
			args:    []string{"output"},
			cfg:     &config.Config{},
			wantErr: `unknown command "output" for "list"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config:    func() (*config.Config, error) { return tt.cfg, tt.cfgErr },
			}

			cmd := NewCmdList(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(ios.ErrOut)
			cmd.SetErr(ios.ErrOut)
			err := cmd.Execute()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
