package copiacmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/build"
	"github.com/qubernetic/copia-cli/internal/config"
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
		"api", "search", "org", "notification", "browse", "completion", "config", "alias",
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

func TestMain_ResolvesOutputFormat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("COPIA_OUTPUT", "yaml")

	code, stdout, stderr := runMain(t, "issue", "list", "-R", "owner/repo")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "Error: invalid COPIA_OUTPUT \"yaml\": valid values are table, json\n", stderr)
}

func TestExecute_DefaultOutputFormat(t *testing.T) {
	t.Setenv("COPIA_OUTPUT", "json")
	ios, _, stdout, _ := iostreams.Test()
	f := NewFactory(ios)
	f.Config = func() (*config.Config, error) { return &config.Config{}, nil }

	err := Execute(f, []string{"issue", "list", "-R", "owner/repo", "--jq", "."})
	assert.EqualError(t, err, "no host configured. Run 'copia auth login' first")
	assert.Empty(t, stdout.String())
}

func TestExecute_SetsPager(t *testing.T) {
	t.Setenv("PAGER", "more")
	t.Setenv("COPIA_PAGER", "")
	require.NoError(t, os.Unsetenv("COPIA_PAGER"))
	ios, _, _, _ := iostreams.Test()
	f := NewFactory(ios)
	f.Config = func() (*config.Config, error) { return &config.Config{Pager: "less -R"}, nil }

	require.NoError(t, Execute(f, []string{"config", "get", "pager"}))
	assert.Equal(t, "less -R", ios.GetPager())
}

func TestExecute_Alias(t *testing.T) {
	tests := []struct {
		name    string
		aliases map[string]string
		args    []string
		want    string
		wantErr string
	}{
		{name: "expansion", aliases: map[string]string{"cg": "config get pager"}, args: []string{"cg"}, want: "less\n"},
		{name: "extra arguments", aliases: map[string]string{"cg": "config get"}, args: []string{"cg", "editor"}, want: "vim\n"},
		{name: "placeholder", aliases: map[string]string{"cg": "config get $1"}, args: []string{"cg", "editor"}, want: "vim\n"},
		{name: "a built-in command wins", aliases: map[string]string{"config": "!echo shadowed"}, args: []string{"config", "get", "pager"}, want: "less\n"},
		{name: "help wins", aliases: map[string]string{"help": "!echo shadowed"}, args: []string{"help", "config"}},
		{name: "shell alias", aliases: map[string]string{"hi": `!printf '%s|%s\n' "$1" "$2"`}, args: []string{"hi", "a b", "$(echo c)"}, want: "a b|$(echo c)\n"},
		{name: "missing argument", aliases: map[string]string{"cg": "config get $1"}, args: []string{"cg"}, wantErr: "not enough arguments for alias: config get $1"},
		{name: "unknown command", aliases: map[string]string{"cg": "config get"}, args: []string{"nope"}, wantErr: `unknown command "nope" for "copia-cli"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			f := NewFactory(ios)
			f.Config = func() (*config.Config, error) {
				return &config.Config{Pager: "less", Editor: "vim", Aliases: tt.aliases}, nil
			}

			err := Execute(f, tt.args)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, stdout.String(), tt.want)
			assert.NotContains(t, stdout.String(), "shadowed")
		})
	}
}

func TestExecute_AliasConfigError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := NewFactory(ios)
	f.Config = func() (*config.Config, error) { return nil, errors.New("parsing config: bad") }

	assert.EqualError(t, Execute(f, []string{"cg"}), "parsing config: bad")
	require.NoError(t, Execute(f, []string{"--version"}))
}

func TestMain_ShellAliasExitCode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := &config.Config{Aliases: map[string]string{"fail": "!echo out; echo err >&2; exit 3"}}
	require.NoError(t, config.Save(filepath.Join(dir, "copia", "config.yml"), cfg))

	code, stdout, stderr := runMain(t, "fail")
	assert.Equal(t, 3, code)
	assert.Equal(t, "out\n", stdout)
	assert.Equal(t, "err\n", stderr)
}
