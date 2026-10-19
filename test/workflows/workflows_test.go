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
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
	With struct {
		Script string `yaml:"script"`
	} `yaml:"with"`
}

// actionFile covers both a workflow (jobs) and a composite action (runs).
type actionFile struct {
	Jobs map[string]struct {
		Steps []step `yaml:"steps"`
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

// TestReleaseVersionIsChecked keeps the release tag out of PKGBUILD, .SRCINFO
// and the RPM spec unless it passed script/check-release-version.sh.
func TestReleaseVersionIsChecked(t *testing.T) {
	for _, file := range []string{"aur.yml", "copr.yml"} {
		run := stepRun(t, file, "Extract version")
		guard := strings.Index(run, `script/check-release-version.sh "$VERSION"`)
		output := strings.Index(run, "GITHUB_OUTPUT")
		if guard < 0 {
			t.Errorf("%s: Extract version does not run script/check-release-version.sh \"$VERSION\"", file)
		} else if output < guard {
			t.Errorf("%s: Extract version writes GITHUB_OUTPUT before it runs the guard", file)
		}
	}
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

// TestChecksumsAreSelectedByName keeps aur.yml from reading a checksum any
// other way than through script/select-checksum.sh with the exact asset name.
func TestChecksumsAreSelectedByName(t *testing.T) {
	run := stepRun(t, "aur.yml", "Get checksums")
	for v, arch := range map[string]string{"SHA_X86": "amd64", "SHA_ARM": "arm64"} {
		want := v + `="$(script/select-checksum.sh checksums.txt "copia-cli_${VERSION}_linux_` + arch + `.tar.gz")"`
		if !strings.Contains(run, want) {
			t.Errorf("aur.yml: Get checksums does not set %s with %s", v, want)
		}
	}
	for _, line := range strings.Split(run, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gh release download") {
			continue
		}
		if strings.Contains(line, "checksums.txt") && !strings.Contains(line, "script/select-checksum.sh checksums.txt ") {
			t.Errorf("aur.yml: Get checksums reads checksums.txt outside script/select-checksum.sh: %s", line)
		}
	}
}

// TestChecksumsAreChecked keeps the checksums out of PKGBUILD and .SRCINFO
// unless they passed script/check-sha256.sh.
func TestChecksumsAreChecked(t *testing.T) {
	run := stepRun(t, "aur.yml", "Get checksums")
	for _, v := range []string{"SHA_X86", "SHA_ARM"} {
		guard := strings.Index(run, `script/check-sha256.sh "$`+v+`"`)
		output := strings.Index(run, `=$`+v+`" >> "$GITHUB_OUTPUT"`)
		if guard < 0 {
			t.Errorf("aur.yml: Get checksums does not run script/check-sha256.sh \"$%s\"", v)
		} else if output < guard {
			t.Errorf("aur.yml: Get checksums does not write $%s to GITHUB_OUTPUT after the guard", v)
		}
	}
}

// TestGoVersionsAreChecked keeps every version bump-go.yml writes to
// GITHUB_OUTPUT, the one from go.dev and the one from go.mod, out of the
// branch name and the PR unless it passed script/check-go-version.sh.
func TestGoVersionsAreChecked(t *testing.T) {
	run := stepRun(t, "bump-go.yml", "Check latest Go version")
	vars := regexp.MustCompile(`=\$([A-Z_]+)" >> "\$GITHUB_OUTPUT"`).FindAllStringSubmatch(run, -1)
	if len(vars) == 0 {
		t.Fatal("bump-go.yml: Check latest Go version writes no shell variable to GITHUB_OUTPUT")
	}
	output := strings.Index(run, "GITHUB_OUTPUT")
	for _, m := range vars {
		guard := strings.Index(run, `script/check-go-version.sh "$`+m[1]+`"`)
		if guard < 0 {
			t.Errorf("bump-go.yml: Check latest Go version does not run script/check-go-version.sh \"$%s\"", m[1])
		} else if output < guard {
			t.Errorf("bump-go.yml: Check latest Go version writes GITHUB_OUTPUT before it checks $%s", m[1])
		}
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

// TestCheckGoVersion covers the guard bump-go.yml runs on the versions it reads
// from go.dev and go.mod before writing them to GITHUB_OUTPUT.
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
