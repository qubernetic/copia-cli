package set

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hostsYAML = "hosts:\n    app.copia.io:\n        token: secret\n        user: john\n"

// setup points the config path at a temporary XDG_CONFIG_HOME and writes
// content there with mode 0600.
func setup(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "copia", "config.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

// runSet runs `alias set` under a root command that has issue list and
// pr checkout, the way copia-cli has them.
func runSet(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	ios, in, stdout, _ := iostreams.Test()
	in.WriteString(stdin)
	noop := func(*cobra.Command, []string) {}

	root := &cobra.Command{Use: "copia-cli", SilenceErrors: true, SilenceUsage: true}
	issue := &cobra.Command{Use: "issue"}
	issue.AddCommand(&cobra.Command{Use: "list", Aliases: []string{"ls"}, Run: noop})
	pr := &cobra.Command{Use: "pr"}
	pr.AddCommand(&cobra.Command{Use: "checkout", Run: noop})
	alias := &cobra.Command{Use: "alias"}
	alias.AddCommand(NewCmdSet(&cmdutil.Factory{IOStreams: ios}))
	root.AddCommand(issue, pr, alias)

	root.SetArgs(append([]string{"alias", "set"}, args...))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err := root.Execute()
	return stdout.String(), err
}

func loadAliases(t *testing.T, path string) map[string]string {
	t.Helper()
	cfg, err := config.Load(path)
	require.NoError(t, err)
	return cfg.Aliases
}

func TestSet(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		stdin    string
		args     []string
		want     map[string]string
		wantOut  string
		wantErr  string
	}{
		{
			name:    "simple",
			args:    []string{"il", "issue list -L 1"},
			want:    map[string]string{"il": "issue list -L 1"},
			wantOut: "Added alias il\n",
		},
		{
			name:    "placeholders",
			args:    []string{"ilabel", "issue list --label $1"},
			want:    map[string]string{"ilabel": "issue list --label $1"},
			wantOut: "Added alias ilabel\n",
		},
		{
			name:    "a command alias in the expansion",
			args:    []string{"il", "issue ls"},
			want:    map[string]string{"il": "issue ls"},
			wantOut: "Added alias il\n",
		},
		{
			name:    "shell prefix",
			args:    []string{"igrep", "!copia-cli issue list | grep $1"},
			want:    map[string]string{"igrep": "!copia-cli issue list | grep $1"},
			wantOut: "Added alias igrep\n",
		},
		{
			name:    "shell flag",
			args:    []string{"--shell", "igrep", "copia-cli issue list | grep $1"},
			want:    map[string]string{"igrep": "!copia-cli issue list | grep $1"},
			wantOut: "Added alias igrep\n",
		},
		{
			name:    "shell short flag with a prefix",
			args:    []string{"-s", "hi", "!echo hi"},
			want:    map[string]string{"hi": "!echo hi"},
			wantOut: "Added alias hi\n",
		},
		{
			name:    "expansion from stdin",
			stdin:   "pr checkout\n",
			args:    []string{"co", "-"},
			want:    map[string]string{"co": "pr checkout"},
			wantOut: "Added alias co\n",
		},
		{
			name:     "existing alias",
			existing: "aliases:\n    co: pr checkout\n",
			args:     []string{"co", "issue list"},
			want:     map[string]string{"co": "pr checkout"},
			wantErr:  "could not create alias co: name already taken; pass --clobber to overwrite it",
		},
		{
			name:     "clobber",
			existing: "aliases:\n    co: pr checkout\n",
			args:     []string{"--clobber", "co", "issue list"},
			want:     map[string]string{"co": "issue list"},
			wantOut:  "Changed alias co\n",
		},
		{
			name:    "shadowing a command",
			args:    []string{"issue", "pr checkout"},
			wantErr: "could not create alias issue: already a copia-cli command",
		},
		{
			name:    "shadowing the alias command",
			args:    []string{"alias", "pr checkout"},
			wantErr: "could not create alias alias: already a copia-cli command",
		},
		{
			name:    "shadowing help",
			args:    []string{"help", "pr checkout"},
			wantErr: "could not create alias help: already a copia-cli command",
		},
		{
			name:    "clobber does not allow shadowing",
			args:    []string{"--clobber", "pr", "issue list"},
			wantErr: "could not create alias pr: already a copia-cli command",
		},
		{
			name:    "a name with a blank",
			args:    []string{"i l", "issue list"},
			wantErr: `could not create alias i l: a name may not be empty, start with - or contain blanks`,
		},
		{
			name:    "an expansion that is no command",
			args:    []string{"x", "nope list"},
			wantErr: "could not create alias x: expansion does not correspond to a copia-cli command",
		},
		{
			name:    "an unparsable expansion",
			args:    []string{"x", `issue list "open`},
			wantErr: "could not create alias x: expansion does not correspond to a copia-cli command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := setup(t, hostsYAML+tt.existing)

			out, err := runSet(t, tt.stdin, tt.args...)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, out)
			assert.Equal(t, tt.want, loadAliases(t, path))

			cfg, err := config.Load(path)
			require.NoError(t, err)
			assert.Equal(t, "secret", cfg.Hosts["app.copia.io"].Users["john"].Token)
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
		})
	}
}

func TestSet_Args(t *testing.T) {
	setup(t, hostsYAML)
	_, err := runSet(t, "", "il")
	assert.EqualError(t, err, "accepts 2 arg(s), received 1")
}

func TestSet_ConfigError(t *testing.T) {
	setup(t, "hosts: [\n")
	_, err := runSet(t, "", "il", "issue list")
	assert.ErrorContains(t, err, "parsing config")
}
