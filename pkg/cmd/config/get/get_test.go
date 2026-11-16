package get

import (
	"errors"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdGet(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		cfg     *config.Config
		cfgErr  error
		wantOut string
		wantErr string
	}{
		{
			name:    "set key",
			args:    []string{"browser"},
			cfg:     &config.Config{Browser: "firefox --new-tab"},
			wantOut: "firefox --new-tab\n",
		},
		{
			name:    "default of an unset key",
			args:    []string{"output"},
			cfg:     &config.Config{},
			wantOut: "table\n",
		},
		{
			name: "unset key without default prints nothing",
			args: []string{"pager"},
			cfg:  &config.Config{},
		},
		{
			name:    "unknown key",
			args:    []string{"token"},
			cfg:     &config.Config{},
			wantErr: `unknown config key "token"; known keys: output, browser, pager, editor`,
		},
		{
			name:    "unreadable config",
			args:    []string{"output"},
			cfgErr:  errors.New("parsing config: bad yaml"),
			wantErr: "parsing config: bad yaml",
		},
		{
			name:    "no key",
			cfg:     &config.Config{},
			wantErr: "accepts 1 arg(s), received 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config:    func() (*config.Config, error) { return tt.cfg, tt.cfgErr },
			}

			cmd := NewCmdGet(f)
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
