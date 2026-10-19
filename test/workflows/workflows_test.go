// Package workflows checks the GitHub Actions workflows under .github/workflows
// and the composite actions under .github/actions.
package workflows

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type step struct {
	Name            string            `yaml:"name"`
	ID              string            `yaml:"id"`
	If              string            `yaml:"if"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	Shell           string            `yaml:"shell"`
	ContinueOnError any               `yaml:"continue-on-error"`
	Env             map[string]string `yaml:"env"`
	With            struct {
		Script string `yaml:"script"`
	} `yaml:"with"`
}

// defaults is the defaults: key of a workflow or a job.
type defaults struct {
	Run struct {
		Shell string `yaml:"shell"`
	} `yaml:"run"`
}

// actionFile covers both a workflow (jobs) and a composite action (runs).
type actionFile struct {
	Defaults defaults `yaml:"defaults"`
	Jobs     map[string]struct {
		Defaults defaults `yaml:"defaults"`
		Steps    []step   `yaml:"steps"`
	} `yaml:"jobs"`
	Runs struct {
		Steps []step `yaml:"steps"`
	} `yaml:"runs"`
}

// scriptExpressions returns one message per step of a workflow or composite
// action whose script source contains `${{`: a `run:` block, or the `script:`
// input of actions/github-script.
func scriptExpressions(file string, data []byte) ([]string, error) {
	var f actionFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}

	var offenders []string
	check := func(where string, steps []step) {
		for i, s := range steps {
			if strings.Contains(s.Run, "${{") {
				offenders = append(offenders, fmt.Sprintf("%s: %s step %d (%q): run block contains ${{; pass the value through env", file, where, i+1, s.Name))
			}
			if isGitHubScript(s.Uses) && strings.Contains(s.With.Script, "${{") {
				offenders = append(offenders, fmt.Sprintf("%s: %s step %d (%q): github-script script contains ${{; pass the value through env", file, where, i+1, s.Name))
			}
		}
	}

	jobNames := make([]string, 0, len(f.Jobs))
	for name := range f.Jobs {
		jobNames = append(jobNames, name)
	}
	sort.Strings(jobNames)
	for _, name := range jobNames {
		check(fmt.Sprintf("job %q", name), f.Jobs[name].Steps)
	}
	check("runs", f.Runs.Steps)
	return offenders, nil
}

func isGitHubScript(uses string) bool {
	return strings.HasPrefix(strings.ToLower(uses), "actions/github-script@")
}

// TestScriptsHaveNoExpressions keeps values from outside the workflow file
// out of script source: a `${{ }}` inside `run:` or a github-script `script:`
// is pasted into the script before the shell or Node parses it. Pass the
// value through the step's `env:` and read it from the environment instead.
func TestScriptsHaveNoExpressions(t *testing.T) {
	root := filepath.Join("..", "..", ".github")
	workflows, err := filepath.Glob(filepath.Join(root, "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) == 0 {
		t.Fatal("no workflow files found")
	}
	actions, err := filepath.Glob(filepath.Join(root, "actions", "*", "action.y*ml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range append(workflows, actions...) {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		name, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		offenders, err := scriptExpressions(name, data)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range offenders {
			t.Error(o)
		}
	}
}

func TestScriptExpressions(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want int
	}{
		{"workflow run with expression", `
jobs:
  build:
    steps:
      - run: echo "${{ github.ref_name }}"
`, 1},
		{"workflow github-script with expression", `
jobs:
  build:
    steps:
      - uses: actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd # v8
        with:
          script: core.info('${{ github.event.issue.title }}')
`, 1},
		{"composite action run with expression", `
runs:
  using: composite
  steps:
    - shell: bash
      run: echo "${{ inputs.version }}"
`, 1},
		{"composite action github-script with expression", `
runs:
  using: composite
  steps:
    - uses: actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd # v8
      with:
        script: core.info('${{ inputs.version }}')
`, 1},
		{"expression in env and if", `
jobs:
  build:
    steps:
      - if: ${{ github.event_name == 'push' }}
        env:
          REF: ${{ github.ref_name }}
        run: echo "$REF"
`, 0},
		{"github-script without expression", `
jobs:
  build:
    steps:
      - uses: actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd # v8
        env:
          TITLE: ${{ github.event.issue.title }}
        with:
          script: core.info(process.env.TITLE)
`, 0},
		{"script input of another action", `
jobs:
  build:
    steps:
      - uses: example/other-action@v1
        with:
          script: ${{ github.ref_name }}
`, 0},
		{"composite action with expression in env", `
runs:
  using: composite
  steps:
    - shell: bash
      env:
        VERSION: ${{ inputs.version }}
      run: echo "$VERSION"
`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scriptExpressions("test.yml", []byte(tt.yaml))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Errorf("scriptExpressions() = %q, want %d offenders", got, tt.want)
			}
		})
	}
}

func TestScriptExpressionsRejectsInvalidYAML(t *testing.T) {
	if _, err := scriptExpressions("test.yml", []byte("jobs: [")); err == nil {
		t.Error("scriptExpressions() succeeded on invalid YAML, want error")
	}
}

// loadWorkflow parses one file under .github/workflows.
func loadWorkflow(t *testing.T, name string) actionFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var wf actionFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return wf
}

// stepRun returns the run block of the step with the given name.
func stepRun(t *testing.T, file, step string) string {
	t.Helper()
	for _, job := range loadWorkflow(t, file).Jobs {
		for _, s := range job.Steps {
			if s.Name == step {
				return s.Run
			}
		}
	}
	t.Fatalf("%s: no step named %q", file, step)
	return ""
}

// TestCheckSHA256 covers the guard aur.yml runs on each checksum read from
// checksums.txt before it reaches GITHUB_OUTPUT and PKGBUILD.
func TestCheckSHA256(t *testing.T) {
	script := filepath.Join("..", "..", "script", "check-sha256.sh")
	valid := strings.Repeat("0123456789abcdef", 4)

	t.Run("accepts a sha256", func(t *testing.T) {
		out, err := exec.Command(script, valid).CombinedOutput()
		if err != nil {
			t.Errorf("check-sha256.sh %q: %v\n%s", valid, err, out)
		}
	})

	for _, sum := range []string{"", valid[:63], valid + "0", strings.ToUpper(valid), valid[:63] + "g", valid[:62] + "')", valid + "\n", valid + "\n" + valid} {
		t.Run("rejects "+sum, func(t *testing.T) {
			out, err := exec.Command(script, sum).CombinedOutput()
			if err == nil {
				t.Fatalf("check-sha256.sh %q succeeded, want failure", sum)
			}
			if !strings.Contains(string(out), "::error::not a sha256 checksum") {
				t.Errorf("check-sha256.sh %q: output %q does not name the error", sum, out)
			}
		})
	}

	// Two matching lines in checksums.txt yield a value with a newline, which
	// must not reach the runner log as a line of its own.
	for _, tc := range []struct{ sum, printed string }{
		{valid + "\n", `"` + valid + `%0A"`},
		{valid + "\n" + valid, `"` + valid + `%0A` + valid + `"`},
		{valid + "\n\n" + valid, `"` + valid + `%0A%0A` + valid + `"`},
		{valid + "\n::add-mask::x", `"` + valid + `%0A::add-mask::x"`},
		{valid + "\r", `"` + valid + `%0D"`},
		{valid + "%0A", `"` + valid + `%250A"`},
	} {
		t.Run("escapes "+tc.sum, func(t *testing.T) {
			out, err := exec.Command(script, tc.sum).CombinedOutput()
			if err == nil {
				t.Fatalf("check-sha256.sh %q succeeded, want failure", tc.sum)
			}
			want := "::error::not a sha256 checksum: " + tc.printed + "\n"
			if string(out) != want {
				t.Errorf("check-sha256.sh %q: output %q, want %q", tc.sum, out, want)
			}
		})
	}
}

// TestSelectChecksum covers the script aur.yml uses to read each archive's
// checksum from checksums.txt: only a line whose name equals the asset name
// counts, and anything but exactly one such line fails.
func TestSelectChecksum(t *testing.T) {
	script := filepath.Join("..", "..", "script", "select-checksum.sh")
	amd := "copia-cli_1.2.3_linux_amd64.tar.gz"
	arm := "copia-cli_1.2.3_linux_arm64.tar.gz"
	sumA := strings.Repeat("a", 64)
	sumB := strings.Repeat("b", 64)
	sumC := strings.Repeat("c", 64)
	goreleaser := sumC + "  copia-cli_1.2.3_darwin_amd64.tar.gz\n" +
		sumA + "  " + amd + "\n" +
		sumB + "  " + arm + "\n"

	run := func(t *testing.T, checksums, asset string) (string, error) {
		t.Helper()
		file := filepath.Join(t.TempDir(), "checksums.txt")
		if err := os.WriteFile(file, []byte(checksums), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(script, file, asset).CombinedOutput()
		return string(out), err
	}

	for _, tc := range []struct{ asset, want string }{{amd, sumA}, {arm, sumB}} {
		t.Run("selects "+tc.asset, func(t *testing.T) {
			out, err := run(t, goreleaser, tc.asset)
			if err != nil {
				t.Fatalf("select-checksum.sh %s: %v\n%s", tc.asset, err, out)
			}
			if out != tc.want+"\n" {
				t.Errorf("select-checksum.sh %s: output %q, want %q", tc.asset, out, tc.want+"\n")
			}
		})
	}

	for _, tc := range []struct{ name, checksums string }{
		{"a missing line", sumB + "  " + arm + "\n"},
		{"a duplicated line", goreleaser + sumA + "  " + amd + "\n"},
		{"a line naming both archives", sumC + "  " + amd + "." + arm + "\n" + sumB + "  " + arm + "\n"},
		{"a superset name", sumC + "  " + amd + ".sbom.json\n" + sumB + "  " + arm + "\n"},
		{"another version", sumC + "  copia-cli_1.2.4_linux_amd64.tar.gz\n" + sumB + "  " + arm + "\n"},
		{"a regex match", sumC + "  copia-cli_1.2.3_linux_amd64xtarxgz\n"},
		{"an empty file", ""},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			out, err := run(t, tc.checksums, amd)
			if err == nil {
				t.Fatalf("select-checksum.sh succeeded with output %q, want failure", out)
			}
			want := "::error::expected exactly one checksum line for \"" + amd + "\"\n"
			if out != want {
				t.Errorf("select-checksum.sh: output %q, want %q", out, want)
			}
		})
	}
}

// TestAURKeyIsPrivate keeps the AUR SSH key from existing with the default
// umask, even for the moment before chmod 600.
func TestAURKeyIsPrivate(t *testing.T) {
	run := stepRun(t, "aur.yml", "Publish to AUR")
	want := `(umask 077; printf '%s\n' "$AUR_SSH_PRIVATE_KEY" > ~/.ssh/aur)`
	if !strings.Contains(run, want) {
		t.Errorf("aur.yml: Publish to AUR does not write the key with %s", want)
	}
}

// TestNoKeyscan keeps workflows from trusting whatever host key answers at run
// time; pin the key in a committed known_hosts file instead.
func TestNoKeyscan(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no workflow files found")
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "ssh-keyscan") {
			t.Errorf("%s: contains ssh-keyscan; pin the host key in a committed known_hosts file", filepath.Base(file))
		}
	}
}

// aurEd25519Fingerprint is the AUR host key fingerprint published on
// https://aur.archlinux.org (home page, "SSH fingerprints").
const aurEd25519Fingerprint = "SHA256:RFzBCUItH9LZS0cKB5UE6ceAYhBD5C8GeOBip8Z11+4"

// TestAURHostKeyIsPinned makes aur.yml push only to the host whose key is in
// .github/aur_known_hosts, and keeps that key the one AUR publishes.
func TestAURHostKeyIsPinned(t *testing.T) {
	run := stepRun(t, "aur.yml", "Publish to AUR")
	for _, want := range []string{
		`-o UserKnownHostsFile=$GITHUB_WORKSPACE/.github/aur_known_hosts`,
		`-o StrictHostKeyChecking=yes`,
	} {
		if !strings.Contains(run, want) {
			t.Errorf("aur.yml: Publish to AUR does not pass %s to ssh", want)
		}
	}

	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "aur_known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	var keys int
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "aur.archlinux.org" || fields[1] != "ssh-ed25519" {
			t.Errorf("aur_known_hosts: line %q is not \"aur.archlinux.org ssh-ed25519 <key>\"", line)
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(fields[2])
		if err != nil {
			t.Errorf("aur_known_hosts: key is not base64: %v", err)
			continue
		}
		sum := sha256.Sum256(blob)
		if got := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]); got != aurEd25519Fingerprint {
			t.Errorf("aur_known_hosts: key fingerprint %s, want %s", got, aurEd25519Fingerprint)
		}
		keys++
	}
	if keys != 1 {
		t.Errorf("aur_known_hosts: %d valid keys, want 1", keys)
	}
}

// TestCheckGoVersion covers the guard bump-go.yml runs on the version it reads
// from go.dev before writing it to GITHUB_OUTPUT.
func TestCheckGoVersion(t *testing.T) {
	script := filepath.Join("..", "..", "script", "check-go-version.sh")

	for _, version := range []string{"1.27", "1.26.9", "10.0.12"} {
		t.Run("accepts "+version, func(t *testing.T) {
			out, err := exec.Command(script, version).CombinedOutput()
			if err != nil {
				t.Errorf("check-go-version.sh %q: %v\n%s", version, err, out)
			}
		})
	}

	for _, version := range []string{
		"", "1.27; echo x", "go1.27", "1", "1.27rc1", "1.27.0.1", "1.27\n", "$(id)",
		"01.27", "1.027", "1.27.00", "00000000000000000000000000000001.0", "1.10000",
	} {
		t.Run("rejects "+version, func(t *testing.T) {
			out, err := exec.Command(script, version).CombinedOutput()
			if err == nil {
				t.Fatalf("check-go-version.sh %q succeeded, want failure", version)
			}
			if !strings.Contains(string(out), "::error::not a Go version") {
				t.Errorf("check-go-version.sh %q: output %q does not name the error", version, out)
			}
		})
	}

	// The runner parses every log line as a possible workflow command, so a
	// rejected value must not reach the log with a raw line break or %.
	for _, tc := range []struct{ version, printed string }{
		{"1.27\n", `"1.27%0A"`},
		{"1.27\nx", `"1.27%0Ax"`},
		{"1.27\n::add-mask::x", `"1.27%0A::add-mask::x"`},
		{"1.27\r", `"1.27%0D"`},
		{"1.27%0A", `"1.27%250A"`},
	} {
		t.Run("escapes "+tc.version, func(t *testing.T) {
			out, err := exec.Command(script, tc.version).CombinedOutput()
			if err == nil {
				t.Fatalf("check-go-version.sh %q succeeded, want failure", tc.version)
			}
			want := "::error::not a Go version: " + tc.printed + "\n"
			if string(out) != want {
				t.Errorf("check-go-version.sh %q: output %q, want %q", tc.version, out, want)
			}
		})
	}
}

// TestWorkflowEscape pins the encoding of script/lib/workflow-escape.sh for
// values with more than one line break, CR or %.
func TestWorkflowEscape(t *testing.T) {
	lib := filepath.Join("..", "..", "script", "lib", "workflow-escape.sh")

	for _, tc := range []struct{ in, want string }{
		{"a\n", "a\n"},
		{"a\n\nb\n", "a%0A%0Ab\n"},
		{"a\nb\nc\n", "a%0Ab%0Ac\n"},
		{"a\r\nb\r\nc\n", "a%0D%0Ab%0D%0Ac\n"},
		{"a\rb\rc\n", "a%0Db%0Dc\n"},
		{"a%%b\n", "a%25%25b\n"},
		{"\n\n\n", "%0A%0A\n"},
	} {
		t.Run(fmt.Sprintf("%q", tc.in), func(t *testing.T) {
			cmd := exec.Command("sh", "-c", `. "$1"; workflow_escape`, "sh", lib)
			cmd.Stdin = strings.NewReader(tc.in)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("workflow_escape %q: %v\n%s", tc.in, err, out)
			}
			if string(out) != tc.want {
				t.Errorf("workflow_escape %q: output %q, want %q", tc.in, out, tc.want)
			}
		})
	}
}

// TestCheckScriptsShareEscape keeps the workflow-command encoding in one
// place: every script/check-*.sh sources script/lib/workflow-escape.sh and
// does not carry its own copy. It also runs each guard on a value with every
// line break and a %, so a new guard is checked without a table of its own.
func TestCheckScriptsShareEscape(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join("..", "..", "script", "check-*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("no script/check-*.sh found")
	}
	for _, script := range scripts {
		data, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		name := filepath.Base(script)
		if !strings.Contains(src, `. "$(dirname "$0")/lib/workflow-escape.sh"`) {
			t.Errorf("%s does not source lib/workflow-escape.sh", name)
		}
		if !strings.Contains(src, "| workflow_escape >&2") {
			t.Errorf("%s does not print its error through workflow_escape", name)
		}
		if strings.Contains(src, "%0A") {
			t.Errorf("%s carries its own copy of the encoding", name)
		}

		value := "1\n::add-mask::x\r::add-mask::y\r\n::add-mask::z%0A\n"
		out, err := exec.Command(script, value).CombinedOutput()
		if err == nil {
			t.Errorf("%s %q succeeded, want failure", name, value)
		}
		if !strings.HasPrefix(string(out), "::error::") || strings.Count(string(out), "\n") != 1 ||
			!strings.HasSuffix(string(out), "\n") || strings.Contains(string(out), "\r") {
			t.Errorf("%s %q: output %q, want one ::error:: line", name, value, out)
		}
	}
}

// TestCheckReleaseVersion covers the guard aur.yml and copr.yml run on the
// version taken from the release tag before it reaches GITHUB_OUTPUT, PKGBUILD
// and the RPM spec. Only X.Y.Z passes: makepkg rejects a pkgver and rpmbuild a
// Version with a hyphen, so a pre-release version cannot be packaged.
func TestCheckReleaseVersion(t *testing.T) {
	script := filepath.Join("..", "..", "script", "check-release-version.sh")

	for _, version := range []string{"1.2.3", "10.0.12"} {
		t.Run("accepts "+version, func(t *testing.T) {
			out, err := exec.Command(script, version).CombinedOutput()
			if err != nil {
				t.Errorf("check-release-version.sh %q: %v\n%s", version, err, out)
			}
		})
	}

	for _, version := range []string{"", "1.2.3/x", "1.2.3$(id)", "1.2.3'", "1.2.3\n", "1.2.3\nx", "v1.2.3", "1.2", "1.2.3-", "1.2.3-rc 1", "1.2.3-rc.1", "1.2.3-beta.1", "10.0.12-beta2"} {
		t.Run("rejects "+version, func(t *testing.T) {
			out, err := exec.Command(script, version).CombinedOutput()
			if err == nil {
				t.Fatalf("check-release-version.sh %q succeeded, want failure", version)
			}
			if !strings.Contains(string(out), "::error::not a release version") {
				t.Errorf("check-release-version.sh %q: output %q does not name the error", version, out)
			}
		})
	}

	// The runner parses every log line as a possible workflow command, so a
	// rejected value must not reach the log with a raw line break or %.
	for _, tc := range []struct{ version, printed string }{
		{"1.2.3\n", `"1.2.3%0A"`},
		{"1.2.3\nx", `"1.2.3%0Ax"`},
		{"1.2.3\n\nx", `"1.2.3%0A%0Ax"`},
		{"1.2.3\n::add-mask::x", `"1.2.3%0A::add-mask::x"`},
		{"1.2.3\r", `"1.2.3%0D"`},
		{"1.2.3%0A", `"1.2.3%250A"`},
	} {
		t.Run("escapes "+tc.version, func(t *testing.T) {
			out, err := exec.Command(script, tc.version).CombinedOutput()
			if err == nil {
				t.Fatalf("check-release-version.sh %q succeeded, want failure", tc.version)
			}
			want := "::error::not a release version: " + tc.printed + "\n"
			if string(out) != want {
				t.Errorf("check-release-version.sh %q: output %q, want %q", tc.version, out, want)
			}
		})
	}
}

// runWithOutput runs script with args and env in addition to a fresh, empty
// GITHUB_OUTPUT file, and returns the combined output and the file content.
func runWithOutput(t *testing.T, script string, env []string, args ...string) (out, written string, err error) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "github_output")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, args...)
	cmd.Env = append(append(os.Environ(), "GITHUB_OUTPUT="+file), env...)
	b, err := cmd.CombinedOutput()
	data, readErr := os.ReadFile(file)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(b), string(data), err
}

// TestReleaseVersion covers the script the Extract version steps of aur.yml
// and copr.yml run: it takes the version from the release tag, checks it with
// script/check-release-version.sh and writes it to GITHUB_OUTPUT only if the
// check passed.
func TestReleaseVersion(t *testing.T) {
	script := filepath.Join("..", "..", "script", "release-version.sh")

	for _, tc := range []struct{ ref, want string }{
		{"v1.2.3", "version=1.2.3\n"},
		{"v10.0.12", "version=10.0.12\n"},
	} {
		t.Run("writes "+tc.ref, func(t *testing.T) {
			out, written, err := runWithOutput(t, script, []string{"GITHUB_REF_NAME=" + tc.ref})
			if err != nil {
				t.Fatalf("release-version.sh with %q: %v\n%s", tc.ref, err, out)
			}
			if written != tc.want {
				t.Errorf("release-version.sh with %q wrote %q, want %q", tc.ref, written, tc.want)
			}
		})
	}

	// Every value check-release-version.sh rejects, with the tag prefix.
	for _, ref := range []string{
		"", "v", "vv1.2.3", "v1.2.3/x", "v1.2.3$(id)", "v1.2.3'", "v1.2.3\n", "v1.2.3\nx",
		"v1.2", "v1.2.3-", "v1.2.3-rc 1", "v1.2.3-rc.1", "v1.2.3-beta.1", "v10.0.12-beta2",
		"v1.2.3\nversion=9.9.9",
	} {
		t.Run(fmt.Sprintf("rejects %q", ref), func(t *testing.T) {
			out, written, err := runWithOutput(t, script, []string{"GITHUB_REF_NAME=" + ref})
			if err == nil {
				t.Fatalf("release-version.sh with %q succeeded, want failure", ref)
			}
			if written != "" {
				t.Errorf("release-version.sh with %q wrote %q, want nothing", ref, written)
			}
			if !strings.Contains(out, "::error::not a release version") {
				t.Errorf("release-version.sh with %q: output %q does not name the error", ref, out)
			}
		})
	}

	t.Run("rejects an unset GITHUB_REF_NAME", func(t *testing.T) {
		cmd := exec.Command(script)
		file := filepath.Join(t.TempDir(), "github_output")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GITHUB_OUTPUT=" + file}
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("release-version.sh without GITHUB_REF_NAME succeeded: %s", out)
		}
		if data, _ := os.ReadFile(file); len(data) != 0 {
			t.Errorf("release-version.sh without GITHUB_REF_NAME wrote %q, want nothing", data)
		}
	})
}

// TestReleaseChecksums covers the script the Get checksums step of aur.yml
// runs: it selects both archive checksums from checksums.txt with
// script/select-checksum.sh, checks each with script/check-sha256.sh and
// writes them to GITHUB_OUTPUT only if both passed.
func TestReleaseChecksums(t *testing.T) {
	script := filepath.Join("..", "..", "script", "release-checksums.sh")
	amd := "copia-cli_1.2.3_linux_amd64.tar.gz"
	arm := "copia-cli_1.2.3_linux_arm64.tar.gz"
	sumA := strings.Repeat("0123456789abcdef", 4)
	sumB := strings.Repeat("fedcba9876543210", 4)
	sumC := strings.Repeat("c", 64)
	goreleaser := sumC + "  copia-cli_1.2.3_darwin_amd64.tar.gz\n" +
		sumA + "  " + amd + "\n" +
		sumB + "  " + arm + "\n" +
		sumC + "  copia-cli_1.2.3_windows_amd64.zip\n"

	run := func(t *testing.T, checksums, version string) (string, string, error) {
		t.Helper()
		file := filepath.Join(t.TempDir(), "checksums.txt")
		if err := os.WriteFile(file, []byte(checksums), 0o600); err != nil {
			t.Fatal(err)
		}
		return runWithOutput(t, script, nil, file, version)
	}

	t.Run("writes both checksums", func(t *testing.T) {
		out, written, err := run(t, goreleaser, "1.2.3")
		if err != nil {
			t.Fatalf("release-checksums.sh: %v\n%s", err, out)
		}
		want := "sha256_x86_64=" + sumA + "\nsha256_aarch64=" + sumB + "\n"
		if written != want {
			t.Errorf("release-checksums.sh wrote %q, want %q", written, want)
		}
	})

	for _, tc := range []struct{ name, checksums, version, err string }{
		{"a missing amd64 line", sumB + "  " + arm + "\n", "1.2.3", "expected exactly one checksum line"},
		{"a missing arm64 line", sumA + "  " + amd + "\n", "1.2.3", "expected exactly one checksum line"},
		{"a duplicated amd64 line", goreleaser + sumA + "  " + amd + "\n", "1.2.3", "expected exactly one checksum line"},
		{"a duplicated arm64 line", goreleaser + sumB + "  " + arm + "\n", "1.2.3", "expected exactly one checksum line"},
		{"a short amd64 hash", sumA[:63] + "  " + amd + "\n" + sumB + "  " + arm + "\n", "1.2.3", "not a sha256 checksum"},
		{"an uppercase arm64 hash", sumA + "  " + amd + "\n" + strings.ToUpper(sumB) + "  " + arm + "\n", "1.2.3", "not a sha256 checksum"},
		{"a non-hex arm64 hash", sumA + "  " + amd + "\n" + sumB[:63] + "g  " + arm + "\n", "1.2.3", "not a sha256 checksum"},
		{"another version", goreleaser, "1.2.4", "expected exactly one checksum line"},
		{"an empty file", "", "1.2.3", "expected exactly one checksum line"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			out, written, err := run(t, tc.checksums, tc.version)
			if err == nil {
				t.Fatalf("release-checksums.sh succeeded, want failure")
			}
			if written != "" {
				t.Errorf("release-checksums.sh wrote %q, want nothing", written)
			}
			if !strings.Contains(out, "::error::"+tc.err) {
				t.Errorf("release-checksums.sh: output %q does not contain %q", out, "::error::"+tc.err)
			}
		})
	}

	t.Run("rejects a missing file", func(t *testing.T) {
		out, written, err := runWithOutput(t, script, nil, filepath.Join(t.TempDir(), "absent.txt"), "1.2.3")
		if err == nil {
			t.Fatalf("release-checksums.sh succeeded, want failure")
		}
		if written != "" {
			t.Errorf("release-checksums.sh wrote %q, want nothing", written)
		}
		if !strings.Contains(out, "::error::expected exactly one checksum line") {
			t.Errorf("release-checksums.sh: output %q does not name the error", out)
		}
	})
}

// releaseGate is a step that checks a value and writes it to GITHUB_OUTPUT.
// Its run block is one script call, so what reaches GITHUB_OUTPUT is what
// TestReleaseVersion and TestReleaseChecksums cover.
type releaseGate struct {
	id, name, run string
	env           map[string]string
}

// releaseOutput is an output of a release gate that later steps read from
// the environment variable name.
type releaseOutput struct {
	name, expr string
}

var (
	versionExpr = "${{ steps.version.outputs.version }}"

	versionGate = releaseGate{
		id: "version", name: "Extract version",
		run: "script/release-version.sh",
	}
	checksumsGate = releaseGate{
		id: "checksums", name: "Get checksums",
		run: `script/release-checksums.sh checksums.txt "$VERSION"`,
		env: map[string]string{"VERSION": versionExpr},
	}

	versionOutput = releaseOutput{"VERSION", versionExpr}
	x86Output     = releaseOutput{"SHA_X86", "${{ steps.checksums.outputs.sha256_x86_64 }}"}
	armOutput     = releaseOutput{"SHA_ARM", "${{ steps.checksums.outputs.sha256_aarch64 }}"}
)

// releaseGates lists, per release workflow, its gates, the outputs its steps
// read, and the steps that must read each output (so the check cannot pass
// by finding no reader at all).
var releaseGates = map[string]struct {
	gates   []releaseGate
	outputs []releaseOutput
	readers map[string][]string
}{
	"aur.yml": {
		gates:   []releaseGate{versionGate, checksumsGate},
		outputs: []releaseOutput{versionOutput, x86Output, armOutput},
		readers: map[string][]string{
			"VERSION": {"Get checksums", "Generate PKGBUILD and .SRCINFO", "Publish to AUR"},
			"SHA_X86": {"Generate PKGBUILD and .SRCINFO"},
			"SHA_ARM": {"Generate PKGBUILD and .SRCINFO"},
		},
	},
	"copr.yml": {
		gates:   []releaseGate{versionGate},
		outputs: []releaseOutput{versionOutput},
		readers: map[string][]string{"VERSION": {"Build SRPM"}},
	},
}

// checkReleaseGates returns one message per way the workflow file could let
// an unchecked value reach a step that reads a gate's output: a gate whose
// run block is not exactly its script call, a gate or job that can run in a
// shell without -e or carry on after a failure, and a reader that takes the
// variable from anywhere but the gate's output.
func checkReleaseGates(file string, data []byte) ([]string, error) {
	spec, ok := releaseGates[file]
	if !ok {
		return nil, fmt.Errorf("%s: not a release workflow", file)
	}
	var wf actionFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}

	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, file+": "+fmt.Sprintf(format, args...))
	}
	if wf.Defaults.Run.Shell != "" {
		report("defaults.run.shell is set; gates must run in the default shell")
	}

	found := map[string]bool{}
	read := map[string]map[string]bool{}
	for jobName, job := range wf.Jobs {
		if job.Defaults.Run.Shell != "" {
			report("job %q sets defaults.run.shell; gates must run in the default shell", jobName)
		}
		produced := map[string]bool{}
		for _, s := range job.Steps {
			for _, g := range spec.gates {
				if s.ID != g.id {
					continue
				}
				found[g.id] = true
				if s.Name != g.name {
					report("step %q is named %q, want %q", g.id, s.Name, g.name)
				}
				if s.Run != g.run && s.Run != g.run+"\n" {
					report("step %q runs %q, want exactly %q", g.name, s.Run, g.run)
				}
				if s.Shell != "" {
					report("step %q sets shell: %s", g.name, s.Shell)
				}
				if s.ContinueOnError != nil {
					report("step %q sets continue-on-error", g.name)
				}
				if s.If != "" {
					report("step %q sets if: %s", g.name, s.If)
				}
				if !envEqual(s.Env, g.env) {
					report("step %q has env %v, want %v", g.name, s.Env, g.env)
				}
			}
			for _, o := range spec.outputs {
				if !readsVar(s, o.name) {
					continue
				}
				if read[o.name] == nil {
					read[o.name] = map[string]bool{}
				}
				read[o.name][s.Name] = true
				if got, ok := s.Env[o.name]; !ok || got != o.expr {
					report("step %q reads %s, want env %s: %s", s.Name, o.name, o.name, o.expr)
				}
				if assignsVar(s.Run, o.name) {
					report("step %q assigns %s in its run block", s.Name, o.name)
				}
				if gate := gateOf(o.expr); !produced[gate] {
					report("step %q reads %s before step %q writes it", s.Name, o.name, gate)
				}
			}
			produced[s.ID] = true
		}
	}

	for _, g := range spec.gates {
		if !found[g.id] {
			report("no step with id %q (%s)", g.id, g.name)
		}
	}
	for _, o := range spec.outputs {
		for _, name := range spec.readers[o.name] {
			if !read[o.name][name] {
				report("step %q does not read %s", name, o.name)
			}
		}
	}
	sort.Strings(problems)
	return problems, nil
}

// readsVar reports whether step s can see the environment variable name: it
// sets it in env or mentions it in its run block.
func readsVar(s step, name string) bool {
	if _, ok := s.Env[name]; ok {
		return true
	}
	return regexp.MustCompile(`\$\{?` + name + `\b`).MatchString(s.Run)
}

// assignsVar reports whether run assigns the shell variable name.
func assignsVar(run, name string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_$])` + name + `=|\bread\b.*\b` + name + `\b`).MatchString(run)
}

// gateOf returns the step id in a ${{ steps.<id>.outputs.<name> }} expression.
func gateOf(expr string) string {
	return strings.Split(strings.TrimPrefix(expr, "${{ steps."), ".")[0]
}

func envEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// TestReleaseGates keeps the release tag and the checksums out of PKGBUILD,
// .SRCINFO and the RPM spec unless they passed script/release-version.sh and
// script/release-checksums.sh.
func TestReleaseGates(t *testing.T) {
	for file := range releaseGates {
		data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", file))
		if err != nil {
			t.Fatal(err)
		}
		problems, err := checkReleaseGates(file, data)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range problems {
			t.Error(p)
		}
	}
}

// TestReleaseGatesCatchEdits applies, to the real workflow files, each edit
// that would let an unchecked value through, and expects checkReleaseGates
// to report it.
func TestReleaseGatesCatchEdits(t *testing.T) {
	for _, tc := range []struct{ name, file, want, old, new string }{
		{"guard followed by || true", "aur.yml", "step \"Extract version\" runs",
			"run: script/release-version.sh\n", "run: script/release-version.sh || true\n"},
		{"version written from GITHUB_REF_NAME", "copr.yml", "step \"Extract version\" runs",
			"run: script/release-version.sh\n",
			"run: |\n          script/release-version.sh\n          echo \"version=${GITHUB_REF_NAME#v}\" >> \"$GITHUB_OUTPUT\"\n"},
		{"VERSION reassigned after the guard", "aur.yml", "step \"Extract version\" runs",
			"run: script/release-version.sh\n",
			"run: |\n          VERSION=x\n          script/release-version.sh\n"},
		{"shell without -e on the gate", "aur.yml", "step \"Extract version\" sets shell",
			"        id: version\n", "        id: version\n        shell: bash {0}\n"},
		{"shell without -e as the job default", "copr.yml", "job \"copr\" sets defaults.run.shell",
			"    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash {0}\n"},
		{"shell without -e as the workflow default", "aur.yml", "defaults.run.shell is set",
			"permissions:\n", "defaults:\n  run:\n    shell: bash {0}\n\npermissions:\n"},
		{"continue-on-error on the gate", "aur.yml", "step \"Get checksums\" sets continue-on-error",
			"        id: checksums\n", "        id: checksums\n        continue-on-error: true\n"},
		{"gate skipped", "copr.yml", "step \"Extract version\" sets if",
			"        id: version\n", "        id: version\n        if: false\n"},
		{"Build SRPM reading the tag", "copr.yml", "step \"Build SRPM\" reads VERSION, want env",
			"          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n          VERSION: ${{ steps.version.outputs.version }}\n",
			"          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n          VERSION: ${{ github.ref_name }}\n"},
		{"Build SRPM assigning VERSION", "copr.yml", "step \"Build SRPM\" assigns VERSION",
			"          sed -i \"s/^Version", "          VERSION=\"${GITHUB_REF_NAME#v}\"\n          sed -i \"s/^Version"},
		{"Build SRPM without VERSION in env", "copr.yml", "step \"Build SRPM\" reads VERSION, want env",
			"          VERSION: ${{ steps.version.outputs.version }}\n\n      - name: Submit", "\n      - name: Submit"},
		{"second unchecked checksum write", "aur.yml", "step \"Get checksums\" runs",
			"run: script/release-checksums.sh checksums.txt \"$VERSION\"\n",
			"run: |\n          script/release-checksums.sh checksums.txt \"$VERSION\"\n          echo \"sha256_x86_64=x\" >> \"$GITHUB_OUTPUT\"\n"},
		{"checksums gate given another VERSION", "aur.yml", "step \"Get checksums\" reads VERSION, want env",
			"          VERSION: ${{ steps.version.outputs.version }}\n\n      - name: Generate",
			"          VERSION: ${{ github.ref_name }}\n\n      - name: Generate"},
		{"PKGBUILD reading another checksum", "aur.yml", "step \"Generate PKGBUILD and .SRCINFO\" reads SHA_ARM, want env",
			"SHA_ARM: ${{ steps.checksums.outputs.sha256_aarch64 }}", "SHA_ARM: ${{ steps.checksums.outputs.sha256_x86_64 }}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), tc.old) != 1 {
				t.Fatalf("%s: %q does not occur exactly once", tc.file, tc.old)
			}
			edited := strings.Replace(string(data), tc.old, tc.new, 1)
			problems, err := checkReleaseGates(tc.file, []byte(edited))
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range problems {
				if strings.Contains(p, tc.want) {
					return
				}
			}
			t.Errorf("%s with %s: problems %q, want one containing %q", tc.file, tc.name, problems, tc.want)
		})
	}
}
