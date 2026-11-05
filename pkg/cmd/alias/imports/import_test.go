package imports

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

type result struct {
	stdout, stderr string
	err            error
}

// runImport runs `alias import` under a root command that has issue list
// and pr checkout, with stdin as standard input.
func runImport(t *testing.T, stdin string, stdinTTY bool, args ...string) result {
	t.Helper()
	ios, in, stdout, stderr := iostreams.Test()
	in.WriteString(stdin)
	ios.SetStdinTTY(stdinTTY)
	noop := func(*cobra.Command, []string) {}

	root := &cobra.Command{Use: "copia-cli", SilenceErrors: true, SilenceUsage: true}
	issue := &cobra.Command{Use: "issue"}
	issue.AddCommand(&cobra.Command{Use: "list", Run: noop})
	pr := &cobra.Command{Use: "pr"}
	pr.AddCommand(&cobra.Command{Use: "checkout", Run: noop})
	alias := &cobra.Command{Use: "alias"}
	alias.AddCommand(NewCmdImport(&cmdutil.Factory{IOStreams: ios}))
	root.AddCommand(issue, pr, alias)

	root.SetArgs(append([]string{"alias", "import"}, args...))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err := root.Execute()
	return result{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func TestImport(t *testing.T) {
	tests := []struct {
		name       string
		existing   string
		file       string
		stdin      string
		stdinTTY   bool
		args       []string
		want       map[string]string
		wantOut    string
		wantStderr string
		wantErr    string
	}{
		{
			name:    "from a file",
			file:    "il: issue list -L 1\nco: pr checkout\nigrep: '!copia-cli issue list | grep $1'\n",
			want:    map[string]string{"il": "issue list -L 1", "co": "pr checkout", "igrep": "!copia-cli issue list | grep $1"},
			wantOut: "Added alias co\nAdded alias igrep\nAdded alias il\n",
		},
		{
			name:    "from stdin",
			stdin:   "co: pr checkout\n",
			args:    []string{"-"},
			want:    map[string]string{"co": "pr checkout"},
			wantOut: "Added alias co\n",
		},
		{
			name:    "from stdin without an argument",
			stdin:   "co: pr checkout\n",
			want:    map[string]string{"co": "pr checkout"},
			wantOut: "Added alias co\n",
		},
		{
			name:     "no argument and a terminal on stdin",
			stdinTTY: true,
			wantErr:  "no filename passed and nothing on standard input",
		},
		{
			name:       "existing aliases are kept",
			existing:   "aliases:\n    co: pr checkout\n",
			file:       "co: issue list\nil: issue list -L 1\n",
			want:       map[string]string{"co": "pr checkout", "il": "issue list -L 1"},
			wantOut:    "Added alias il\n",
			wantStderr: "could not import alias co: name already taken; pass --clobber to overwrite it\n",
			wantErr:    "1 of 2 aliases not imported",
		},
		{
			name:     "clobber",
			existing: "aliases:\n    co: pr checkout\n",
			file:     "co: issue list\nil: issue list -L 1\n",
			args:     []string{"--clobber"},
			want:     map[string]string{"co": "issue list", "il": "issue list -L 1"},
			wantOut:  "Changed alias co\nAdded alias il\n",
		},
		{
			name:       "shadowing and invalid aliases are refused",
			file:       "issue: pr checkout\nx: nope\n'-y': issue list\nok: issue list\n",
			args:       []string{"--clobber"},
			want:       map[string]string{"ok": "issue list"},
			wantOut:    "Added alias ok\n",
			wantStderr: "could not import alias -y: a name may not be empty, start with - or contain blanks\ncould not import alias issue: already a copia-cli command\ncould not import alias x: expansion does not correspond to a copia-cli command\n",
			wantErr:    "3 of 4 aliases not imported",
		},
		{
			name:    "not a map of strings",
			file:    "co:\n    nested: pr checkout\n",
			wantErr: "could not parse aliases.yml",
		},
		{name: "an empty file", file: "\n"},
		{name: "a missing file", args: []string{"missing.yml"}, wantErr: "could not read missing.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			path := filepath.Join(dir, "copia", "config.yml")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
			require.NoError(t, os.WriteFile(path, []byte(hostsYAML+tt.existing), 0600))
			args := tt.args
			if tt.file != "" {
				t.Chdir(dir)
				require.NoError(t, os.WriteFile("aliases.yml", []byte(tt.file), 0600))
				args = append([]string{"aliases.yml"}, args...)
			} else if len(args) > 0 && args[0] == "missing.yml" {
				t.Chdir(dir)
			}

			got := runImport(t, tt.stdin, tt.stdinTTY, args...)
			if tt.wantErr != "" {
				assert.ErrorContains(t, got.err, tt.wantErr)
			} else {
				require.NoError(t, got.err)
			}
			assert.Equal(t, tt.wantOut, got.stdout)
			assert.Equal(t, tt.wantStderr, got.stderr)

			cfg, err := config.Load(path)
			require.NoError(t, err)
			wantAliases := tt.want
			if wantAliases == nil && tt.existing != "" {
				wantAliases = map[string]string{"co": "pr checkout"}
			}
			assert.Equal(t, wantAliases, cfg.Aliases)
			assert.Equal(t, "secret", cfg.Hosts["app.copia.io"].Users["john"].Token)
		})
	}
}

func TestImport_ConfigError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "copia", "config.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("hosts: [\n"), 0600))

	got := runImport(t, "co: pr checkout\n", false, "-")
	assert.ErrorContains(t, got.err, "parsing config")
}
