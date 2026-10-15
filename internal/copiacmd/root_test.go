package copiacmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/build"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootCmd_RegistersCommands(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewRootCmd(&cmdutil.Factory{IOStreams: ios})

	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	for _, want := range []string{
		"auth", "repo", "issue", "label", "pr", "release",
		"api", "search", "org", "notification", "completion",
	} {
		assert.Contains(t, names, want)
	}

	assert.Equal(t, "copia-cli", cmd.Name())
	assert.Equal(t, build.Version, cmd.Version)
	assert.True(t, cmd.SilenceErrors)
	assert.True(t, cmd.SilenceUsage)
}

func TestNewRootCmd_PersistentFlags(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewRootCmd(f)

	repoFlag := cmd.PersistentFlags().ShorthandLookup("R")
	require.NotNil(t, repoFlag)
	assert.Equal(t, "repo", repoFlag.Name)
	require.NotNil(t, cmd.PersistentFlags().Lookup("host"))
	require.NotNil(t, cmd.PersistentFlags().Lookup("token"))

	cmd.SetArgs([]string{"--host", "copia.example.com", "--token", "secret", "-R", "my-org/my-repo", "completion", "bash"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "copia.example.com", f.Host)
	assert.Equal(t, "secret", f.Token)
	assert.Equal(t, "my-org/my-repo", f.Repo)
	assert.Contains(t, stdout.String(), "bash completion")
}

func TestNewRootCmd_Version(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewRootCmd(&cmdutil.Factory{IOStreams: ios})

	out := &stringWriter{}
	cmd.SetArgs([]string{"--version"})
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, build.VersionInfo()+"\n", out.s)
}

type stringWriter struct{ s string }

func (w *stringWriter) Write(p []byte) (int, error) {
	w.s += string(p)
	return len(p), nil
}

// runMain runs Main with the given arguments, capturing stdout and stderr in
// temp files because iostreams.System binds to the os.Std* files.
func runMain(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)

	origArgs, origOut, origErr := os.Args, os.Stdout, os.Stderr
	t.Cleanup(func() {
		os.Args, os.Stdout, os.Stderr = origArgs, origOut, origErr
		_ = outFile.Close()
		_ = errFile.Close()
	})
	os.Args = append([]string{"copia-cli"}, args...)
	os.Stdout, os.Stderr = outFile, errFile

	code = Main()

	os.Stdout, os.Stderr = origOut, origErr
	outBytes, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	errBytes, err := os.ReadFile(errFile.Name())
	require.NoError(t, err)
	return code, string(outBytes), string(errBytes)
}

func TestMain_Version(t *testing.T) {
	code, stdout, _ := runMain(t, "--version")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "copia-cli version")
}

func TestMain_UnknownCommand(t *testing.T) {
	code, _, stderr := runMain(t, "no-such-command")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "Error: unknown command \"no-such-command\"")
}

func TestMain_LoadsConfigFromDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("COPIA_TOKEN", "")
	t.Setenv("COPIA_HOST", "")

	code, stdout, stderr := runMain(t, "notification", "list")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "Error: no host configured")
}
