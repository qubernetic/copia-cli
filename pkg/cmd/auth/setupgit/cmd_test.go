package setupgit

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCmd(t *testing.T, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := NewCmdSetupGit(f)
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestNewCmdSetupGit(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	ours := "!" + shellQuote(exe) + " auth git-credential"
	tests := []struct {
		name       string
		args       []string
		rootHost   string // the --host flag of the root command
		configured []string
		wantErr    string
	}{
		{name: "every host", configured: []string{"a.example.com", "b.example.com"}},
		{name: "host flag", args: []string{"--host", "b.example.com"}, configured: []string{"b.example.com"}},
		{name: "host flag of the root command", rootHost: "a.example.com", configured: []string{"a.example.com"}},
		{name: "force", args: []string{"--host", "c.example.com", "--force"}, configured: []string{"c.example.com"}},
		{name: "force shorthand", args: []string{"--host", "c.example.com", "-f"}, configured: []string{"c.example.com"}},
		{name: "force without host", args: []string{"--force"}, wantErr: "--force must be used with --host"},
		{name: "arguments", args: []string{"a.example.com"}, wantErr: `unknown command "a.example.com" for "setup-git"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitHome(t)
			t.Setenv("PATH", os.Getenv("PATH"))
			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Config: loadConfig(t, twoHosts), Host: tt.rootHost}

			err := runCmd(t, f, tt.args...)

			assert.Empty(t, stdout.String())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			for _, host := range []string{"a.example.com", "b.example.com", "c.example.com"} {
				if slices.Contains(tt.configured, host) {
					assert.Equal(t, []string{"", ours}, helpers(t, host), host)
				} else {
					assert.Nil(t, helpers(t, host), host)
				}
			}
		})
	}
}


// TestExecutable_PrefersThePathEntry checks that a copia-cli in PATH that is
// the running binary (here a symlink to the test binary) is what git runs.
func TestExecutable_PrefersThePathEntry(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	link := filepath.Join(dir, "copia-cli")
	require.NoError(t, os.Symlink(exe, link))
	t.Setenv("PATH", dir)

	got, err := executable()

	require.NoError(t, err)
	assert.Equal(t, link, got)
}

// TestExecutable_IgnoresAnotherBinaryInPath checks that a different
// copia-cli in PATH does not replace the running binary.
func TestExecutable_IgnoresAnotherBinaryInPath(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "copia-cli"), []byte("#!/bin/sh\n"), 0o700))
	t.Setenv("PATH", dir)

	got, err := executable()

	require.NoError(t, err)
	assert.Equal(t, exe, got)
}
