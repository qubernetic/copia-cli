package setupgit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoHosts = `hosts:
  a.example.com:
    user: joe
    users:
      joe:
        token: file-token-joe
  b.example.com:
    user: jane
    users:
      jane: {}
`

// gitHome points HOME and the global git config at a temporary directory and
// returns the path of that global config file.
func gitHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	global := filepath.Join(home, ".gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	return global
}

// helpers returns every credential helper the global git config holds for
// the https URL of host.
func helpers(t *testing.T, host string) []string {
	t.Helper()
	out, err := exec.Command("git", "config", "--global", "--get-all", "credential.https://"+host+".helper").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}

func loadConfig(t *testing.T, content string) func() (*config.Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return func() (*config.Config, error) { return config.Load(path) }
}

func TestSetupGitRun_AllHosts(t *testing.T) {
	gitHome(t)
	ios, _, stdout, stderr := iostreams.Test()

	err := setupGitRun(&SetupGitOptions{
		IO:         ios,
		Config:     loadConfig(t, twoHosts),
		Executable: "/usr/bin/copia-cli",
	})

	require.NoError(t, err)
	want := []string{"", "!'/usr/bin/copia-cli' auth git-credential"}
	assert.Equal(t, want, helpers(t, "a.example.com"))
	assert.Equal(t, want, helpers(t, "b.example.com"))
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestSetupGitRun_Hosts(t *testing.T) {
	const ours = "!'/usr/bin/copia-cli' auth git-credential"
	tests := []struct {
		name       string
		config     string
		host       string
		force      bool
		configured []string // hosts that get the helper
		wantErr    string
	}{
		{
			name:       "one logged-in host",
			config:     twoHosts,
			host:       "b.example.com",
			configured: []string{"b.example.com"},
		},
		{
			name:    "unknown host",
			config:  twoHosts,
			host:    "c.example.com",
			wantErr: "not logged in to c.example.com. Run 'copia-cli auth login --host c.example.com' first, or use --force",
		},
		{
			name:       "unknown host with force",
			config:     twoHosts,
			host:       "c.example.com:8443",
			force:      true,
			configured: []string{"c.example.com:8443"},
		},
		{
			name:       "known host with force",
			config:     twoHosts,
			host:       "a.example.com",
			force:      true,
			configured: []string{"a.example.com"},
		},
		{
			name:    "host entry without an account",
			config:  "hosts:\n  a.example.com:\n    user: \"\"\n",
			host:    "a.example.com",
			wantErr: "not logged in to a.example.com. Run 'copia-cli auth login --host a.example.com' first, or use --force",
		},
		{
			name:    "no host logged in",
			config:  "hosts:\n  a.example.com:\n    user: \"\"\n",
			wantErr: "not logged in to any hosts. Run 'copia-cli auth login' first",
		},
		{
			name:    "empty config",
			config:  "",
			wantErr: "not logged in to any hosts. Run 'copia-cli auth login' first",
		},
		{
			name:    "invalid host with force",
			config:  twoHosts,
			host:    "c.example.com/x",
			force:   true,
			wantErr: `invalid host "c.example.com/x": want a host name with an optional port`,
		},
		{
			name:    "wildcard host with force",
			config:  twoHosts,
			host:    "*.example.com",
			force:   true,
			wantErr: `invalid host "*.example.com": want a host name with an optional port`,
		},
		{
			name:    "host with a user name with force",
			config:  twoHosts,
			host:    "joe@a.example.com",
			force:   true,
			wantErr: `invalid host "joe@a.example.com": want a host name with an optional port`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global := gitHome(t)
			ios, _, stdout, _ := iostreams.Test()

			err := setupGitRun(&SetupGitOptions{
				IO:         ios,
				Config:     loadConfig(t, tt.config),
				Host:       tt.host,
				Force:      tt.force,
				Executable: "/usr/bin/copia-cli",
			})

			assert.Empty(t, stdout.String())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.NoFileExists(t, global)
				return
			}
			require.NoError(t, err)
			for _, host := range []string{"a.example.com", "b.example.com", "c.example.com:8443"} {
				if slices.Contains(tt.configured, host) {
					assert.Equal(t, []string{"", ours}, helpers(t, host), host)
				} else {
					assert.Nil(t, helpers(t, host), host)
				}
			}
		})
	}
}

// fakeHelper writes a credential helper script to dir that appends its name
// and arguments to the file log, and returns its path.
func fakeHelper(t *testing.T, dir, name, log string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho "+name+" \"$@\" >> '"+strings.ReplaceAll(log, "'", `'\''`)+"'\n"), 0o700))
	return path
}

// TestSetupGitRun_ReplacesEarlierHelpers checks, the way git reads it, that
// the empty entry drops the helpers configured before it for the host, under
// the plain key, the key with the default port and a key with a path.
func TestSetupGitRun_ReplacesEarlierHelpers(t *testing.T) {
	global := gitHome(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	earlier := "[credential]\n\thelper = " + fakeHelper(t, dir, "plain", log) + "\n" +
		"[credential \"https://a.example.com:443\"]\n\thelper = " + fakeHelper(t, dir, "port", log) + "\n" +
		"[credential \"https://a.example.com/x/repo.git\"]\n\thelper = " + fakeHelper(t, dir, "path", log) + "\n"
	require.NoError(t, os.WriteFile(global, []byte(earlier), 0o600))
	exe := fakeHelper(t, dir, "copia-cli", log)
	ios, _, _, _ := iostreams.Test()

	err := setupGitRun(&SetupGitOptions{IO: ios, Config: loadConfig(t, twoHosts), Host: "a.example.com", Executable: exe})

	require.NoError(t, err)
	assert.Equal(t, []string{"", "!" + shellQuote(exe) + " auth git-credential"}, helpers(t, "a.example.com"))
	out, err := exec.Command("git", "config", "--global", "--get-all", "credential.helper").Output()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "plain")+"\n", string(out), "the helper of every other URL stays")

	for _, request := range []string{
		"protocol=https\nhost=a.example.com\n\n",
		"protocol=https\nhost=a.example.com\npath=x/repo.git\n\n",
	} {
		require.NoError(t, os.RemoveAll(log))
		fill := exec.Command("git", "-c", "credential.useHttpPath=true", "credential", "fill")
		fill.Stdin = strings.NewReader(request)
		fill.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		_, _ = fill.Output()

		got, err := os.ReadFile(log)
		require.NoError(t, err, "git did not run the helper")
		assert.Equal(t, "copia-cli auth git-credential get\n", string(got), "only the copia-cli helper runs")
	}
}

// TestSetupGitRun_LaterHelpersStay documents what the empty entry does not
// do, as with gh: a helper configured after it still takes part.
func TestSetupGitRun_LaterHelpersStay(t *testing.T) {
	gitHome(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	exe := fakeHelper(t, dir, "copia-cli", log)
	later := fakeHelper(t, dir, "later", log)
	ios, _, _, _ := iostreams.Test()
	require.NoError(t, setupGitRun(&SetupGitOptions{IO: ios, Config: loadConfig(t, twoHosts), Host: "a.example.com", Executable: exe}))

	fill := exec.Command("git", "-c", "credential.helper="+later, "credential", "fill")
	fill.Stdin = strings.NewReader("protocol=https\nhost=a.example.com\n\n")
	fill.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	_, _ = fill.Output()

	got, err := os.ReadFile(log)
	require.NoError(t, err, "git did not run the helpers")
	assert.Equal(t, "copia-cli auth git-credential get\nlater get\n", string(got), "a later helper still runs")
}

func TestSetupGitRun_RerunChangesNothing(t *testing.T) {
	global := gitHome(t)
	ios, _, _, _ := iostreams.Test()
	opts := &SetupGitOptions{IO: ios, Config: loadConfig(t, twoHosts), Executable: "/usr/bin/copia-cli"}

	require.NoError(t, setupGitRun(opts))
	first, err := os.ReadFile(global)
	require.NoError(t, err)
	require.NoError(t, setupGitRun(opts))
	second, err := os.ReadFile(global)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestSetupGitRun_QuotesTheExecutable(t *testing.T) {
	tests := []struct {
		name       string
		executable string
		want       string
	}{
		{name: "spaces", executable: "/opt/Copia CLI/copia-cli", want: "!'/opt/Copia CLI/copia-cli' auth git-credential"},
		{name: "single quote", executable: "/home/o'brien/bin/copia-cli", want: `!'/home/o'\''brien/bin/copia-cli' auth git-credential`},
		{name: "shell characters", executable: "/tmp/$HOME;x/copia-cli", want: "!'/tmp/$HOME;x/copia-cli' auth git-credential"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitHome(t)
			ios, _, _, _ := iostreams.Test()

			err := setupGitRun(&SetupGitOptions{IO: ios, Config: loadConfig(t, twoHosts), Host: "a.example.com", Executable: tt.executable})

			require.NoError(t, err)
			assert.Equal(t, []string{"", tt.want}, helpers(t, "a.example.com"))
		})
	}
}

// TestSetupGitRun_HelperRunsTheExecutable checks the quoting the way git
// reads it: git runs the configured helper through the shell, and the fake
// executable in a directory whose name holds shell syntax writes its
// arguments.
func TestSetupGitRun_HelperRunsTheExecutable(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "space and single quote", dir: "it's a dir"},
		{name: "command substitution", dir: "$(touch pwned)"},
		{name: "backtick", dir: "a`touch pwned`b"},
		{name: "backslash", dir: `a\b\`},
		{name: "double quote", dir: `a"b`},
		{name: "newline", dir: "a\nb"},
		{name: "control operators", dir: "a;b|c&d"},
		{name: "globs", dir: "*?[]"},
		{name: "comment", dir: "a #b"},
		{name: "tilde", dir: "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitHome(t)
			base := t.TempDir()
			dir := filepath.Join(base, tt.dir)
			require.NoError(t, os.MkdirAll(dir, 0o700))
			exe := filepath.Join(dir, "copia-cli")
			args := filepath.Join(base, "args")
			require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\necho \"$@\" > '"+strings.ReplaceAll(args, "'", `'\''`)+"'\n"), 0o700))
			ios, _, _, _ := iostreams.Test()
			require.NoError(t, setupGitRun(&SetupGitOptions{IO: ios, Config: loadConfig(t, twoHosts), Host: "a.example.com", Executable: exe}))

			fill := exec.Command("git", "credential", "fill")
			fill.Dir = base
			fill.Stdin = strings.NewReader("protocol=https\nhost=a.example.com\n\n")
			fill.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			_, _ = fill.Output()

			got, err := os.ReadFile(args)
			require.NoError(t, err, "git did not run the helper")
			assert.Equal(t, "auth git-credential get\n", string(got))
			assert.NoFileExists(t, filepath.Join(base, "pwned"))
		})
	}
}

func TestSetupGitRun_GitFails(t *testing.T) {
	gitHome(t)
	t.Setenv("PATH", t.TempDir())
	ios, _, _, _ := iostreams.Test()

	err := setupGitRun(&SetupGitOptions{IO: ios, Config: loadConfig(t, twoHosts), Executable: "/usr/bin/copia-cli"})

	require.ErrorContains(t, err, "failed to set up the git credential helper for a.example.com: ")
}

func TestSetupGitRun_ConfigError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	err := setupGitRun(&SetupGitOptions{IO: ios, Config: func() (*config.Config, error) { return nil, errors.New("broken config") }})

	require.EqualError(t, err, "broken config")
}
