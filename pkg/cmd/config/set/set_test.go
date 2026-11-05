package set

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hostsYAML = "hosts:\n    app.copia.io:\n        token: secret\n        user: john\n    on-prem.example.com:\n        token: other\n        user: jane\n"

// setup points the config path at a temporary XDG_CONFIG_HOME and writes
// content there with mode 0600, unless content is empty.
func setup(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "copia", "config.yml")
	if content != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	}
	return path
}

func runSet(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ios, _, stdout, _ := iostreams.Test()
	cmd := NewCmdSet(&cmdutil.Factory{IOStreams: ios})
	cmd.SetArgs(args)
	cmd.SetOut(ios.ErrOut)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	return stdout.String(), err
}

func TestNewCmdSet(t *testing.T) {
	tests := []struct {
		args []string
		want config.Config
	}{
		{args: []string{"output", "json"}, want: config.Config{Output: "json"}},
		{args: []string{"browser", "firefox --new-tab"}, want: config.Config{Browser: "firefox --new-tab"}},
		{args: []string{"pager", "less -R"}, want: config.Config{Pager: "less -R"}},
		{args: []string{"editor", "vim"}, want: config.Config{Editor: "vim"}},
	}

	for _, tt := range tests {
		t.Run(tt.args[0], func(t *testing.T) {
			path := setup(t, hostsYAML)

			out, err := runSet(t, tt.args...)
			require.NoError(t, err)
			assert.Empty(t, out)

			loaded, err := config.Load(path)
			require.NoError(t, err)
			value, err := loaded.Get(tt.args[0])
			require.NoError(t, err)
			assert.Equal(t, tt.args[1], value)

			tt.want.Hosts = map[string]*config.HostConfig{
				"app.copia.io":        {Token: "secret", User: "john"},
				"on-prem.example.com": {Token: "other", User: "jane"},
			}
			assert.Equal(t, &tt.want, loaded, "hosts untouched")

			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
		})
	}
}

func TestNewCmdSet_KeepsOtherSettings(t *testing.T) {
	path := setup(t, hostsYAML+"output: json\nbrowser: firefox\n")

	_, err := runSet(t, "editor", "nano")
	require.NoError(t, err)

	loaded, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "json", loaded.Output)
	assert.Equal(t, "firefox", loaded.Browser)
	assert.Equal(t, "nano", loaded.Editor)
	assert.Len(t, loaded.Hosts, 2)
}

func TestNewCmdSet_CreatesFile(t *testing.T) {
	path := setup(t, "")

	_, err := runSet(t, "output", "json")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "json", loaded.Output)
}

func TestNewCmdSet_FileUnchangedOnError(t *testing.T) {
	tests := []struct {
		name    string
		content string
		args    []string
		wantErr string
	}{
		{
			name:    "unknown key",
			content: hostsYAML,
			args:    []string{"token", "abc"},
			wantErr: `unknown config key "token"; known keys: output, browser, pager, editor`,
		},
		{
			name:    "invalid value",
			content: hostsYAML + "output: table\n",
			args:    []string{"output", "yaml"},
			wantErr: `failed to set "output" to "yaml": valid values are table, json`,
		},
		{
			name:    "unreadable config",
			content: "hosts: [\n",
			args:    []string{"output", "json"},
			wantErr: "parsing config: yaml: line 1: did not find expected node content",
		},
		{
			name:    "missing value",
			content: hostsYAML,
			args:    []string{"output"},
			wantErr: "accepts 2 arg(s), received 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := setup(t, tt.content)

			_, err := runSet(t, tt.args...)
			assert.EqualError(t, err, tt.wantErr)

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.content, string(data))
		})
	}
}

func TestNewCmdSet_UnknownKeyCreatesNoFile(t *testing.T) {
	path := setup(t, "")

	_, err := runSet(t, "token", "abc")
	assert.Error(t, err)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}
