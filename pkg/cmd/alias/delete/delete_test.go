package delete

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hostsYAML = "hosts:\n    app.copia.io:\n        token: secret\n        user: john\n"

func TestDelete(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		args     []string
		want     map[string]string
		wantOut  string
		wantErr  string
	}{
		{
			name:     "one alias",
			existing: "aliases:\n    co: pr checkout\n    il: issue list -L 1\n",
			args:     []string{"co"},
			want:     map[string]string{"il": "issue list -L 1"},
			wantOut:  "Deleted alias co; was pr checkout\n",
		},
		{
			name:     "the last alias",
			existing: "aliases:\n    co: pr checkout\n",
			args:     []string{"co"},
			wantOut:  "Deleted alias co; was pr checkout\n",
		},
		{
			name:     "all aliases",
			existing: "aliases:\n    co: pr checkout\n    il: issue list -L 1\n",
			args:     []string{"--all"},
			wantOut:  "Deleted alias co; was pr checkout\nDeleted alias il; was issue list -L 1\n",
		},
		{
			name:     "no such alias",
			existing: "aliases:\n    co: pr checkout\n",
			args:     []string{"il"},
			want:     map[string]string{"co": "pr checkout"},
			wantErr:  "no such alias il",
		},
		{name: "all without aliases", args: []string{"--all"}, wantErr: "no aliases configured"},
		{name: "no argument", args: []string{}, wantErr: "must specify an alias to delete or --all"},
		{name: "a name and --all", args: []string{"co", "--all"}, wantErr: "cannot use --all with an alias name"},
		{name: "two names", args: []string{"co", "il"}, wantErr: "accepts at most 1 arg(s), received 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			path := filepath.Join(dir, "copia", "config.yml")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
			require.NoError(t, os.WriteFile(path, []byte(hostsYAML+tt.existing), 0600))

			ios, _, stdout, _ := iostreams.Test()
			cmd := NewCmdDelete(&cmdutil.Factory{IOStreams: ios})
			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.Execute()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())

			cfg, err := config.Load(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Aliases)
			assert.Equal(t, "secret", cfg.Hosts["app.copia.io"].Users["john"].Token)
		})
	}
}

func TestDelete_ConfigError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "copia", "config.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("hosts: [\n"), 0600))

	ios, _, _, _ := iostreams.Test()
	cmd := NewCmdDelete(&cmdutil.Factory{IOStreams: ios})
	cmd.SetArgs([]string{"co"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	assert.ErrorContains(t, cmd.Execute(), "parsing config")
}
