// Package workflows checks the GitHub Actions workflows under .github/workflows,
// the composite actions under .github/actions and every local action they use.
package workflows

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

// actionFile covers both a workflow (jobs) and an action (runs).
type actionFile struct {
	Jobs map[string]struct {
		Uses  string `yaml:"uses"`
		Steps []step `yaml:"steps"`
	} `yaml:"jobs"`
	Runs struct {
		Using      string   `yaml:"using"`
		Steps      []step   `yaml:"steps"`
		Args       []string `yaml:"args"`
		Entrypoint string   `yaml:"entrypoint"`
	} `yaml:"runs"`
}

func parseActionFile(file string, data []byte) (actionFile, error) {
	var f actionFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("%s: %w", file, err)
	}
	return f, nil
}

func sortedJobs(f actionFile) []string {
	names := make([]string, 0, len(f.Jobs))
	for name := range f.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// scriptExpressions returns one message per place in a workflow or action
// whose script source contains `${{`: a `run:` block, the `script:` input of
// actions/github-script, or a docker action's `runs.args` entry or
// `runs.entrypoint`.
func scriptExpressions(file string, data []byte) ([]string, error) {
	f, err := parseActionFile(file, data)
	if err != nil {
		return nil, err
	}
	return expressions(file, f), nil
}

func expressions(file string, f actionFile) []string {
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

	for _, name := range sortedJobs(f) {
		check(fmt.Sprintf("job %q", name), f.Jobs[name].Steps)
	}
	check("runs", f.Runs.Steps)

	if strings.EqualFold(f.Runs.Using, "docker") {
		for i, arg := range f.Runs.Args {
			if strings.Contains(arg, "${{") {
				offenders = append(offenders, fmt.Sprintf("%s: runs.args entry %d contains ${{; pass the value through env", file, i+1))
			}
		}
		if strings.Contains(f.Runs.Entrypoint, "${{") {
			offenders = append(offenders, fmt.Sprintf("%s: runs.entrypoint contains ${{; pass the value through env", file))
		}
	}
	return offenders
}

func isGitHubScript(uses string) bool {
	return strings.HasPrefix(strings.ToLower(uses), "actions/github-script@")
}

// localUses returns every `uses: ./<path>` of a workflow or action: the steps
// of its jobs and of `runs`, and reusable workflows called by a job.
func localUses(f actionFile) []string {
	var uses []string
	add := func(u string) {
		if strings.HasPrefix(u, "./") {
			uses = append(uses, u)
		}
	}
	for _, name := range sortedJobs(f) {
		add(f.Jobs[name].Uses)
		for _, s := range f.Jobs[name].Steps {
			add(s.Uses)
		}
	}
	for _, s := range f.Runs.Steps {
		add(s.Uses)
	}
	return uses
}

// localTargets resolves `uses: ./<path>` to the files it runs, relative to the
// repository root: the reusable workflow itself, or <path>/action.yml and
// <path>/action.yaml, whichever exist. It returns a message instead when the
// path leaves the repository or names no file.
func localTargets(repo, file, uses string) ([]string, string) {
	dir := path.Clean(uses)
	if dir == ".." || strings.HasPrefix(dir, "../") {
		return nil, fmt.Sprintf("%s: uses %s, which points outside the repository", file, uses)
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(repo, filepath.FromSlash(name)))
		return err == nil
	}

	if ext := path.Ext(dir); ext == ".yml" || ext == ".yaml" {
		if !exists(dir) {
			return nil, fmt.Sprintf("%s: uses %s, but %s does not exist", file, uses, dir)
		}
		return []string{dir}, ""
	}

	var targets []string
	for _, name := range []string{"action.yml", "action.yaml"} {
		if exists(path.Join(dir, name)) {
			targets = append(targets, path.Join(dir, name))
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Sprintf("%s: uses %s, but %s has no action.yml or action.yaml", file, uses, dir)
	}
	return targets, ""
}

// scanRepo checks every workflow under .github/workflows, every action under
// .github/actions/* and every local action or reusable workflow they use,
// following local actions that use local actions in turn. It returns one
// message per expression in script source and per `uses: ./<path>` that names
// no file.
func scanRepo(repo string) ([]string, error) {
	var queue []string
	for _, pattern := range []string{".github/workflows/*.y*ml", ".github/actions/*/action.y*ml"} {
		matches, err := filepath.Glob(filepath.Join(repo, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			rel, err := filepath.Rel(repo, m)
			if err != nil {
				return nil, err
			}
			queue = append(queue, filepath.ToSlash(rel))
		}
	}
	if len(queue) == 0 {
		return nil, errors.New("no workflow files found")
	}

	seen := map[string]bool{}
	var offenders []string
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		if seen[file] {
			continue
		}
		seen[file] = true

		data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		f, err := parseActionFile(file, data)
		if err != nil {
			return nil, err
		}
		offenders = append(offenders, expressions(file, f)...)
		for _, uses := range localUses(f) {
			targets, problem := localTargets(repo, file, uses)
			if problem != "" {
				offenders = append(offenders, problem)
			}
			queue = append(queue, targets...)
		}
	}
	return offenders, nil
}

// TestScriptsHaveNoExpressions keeps values from outside the workflow file
// out of script source: a `${{ }}` inside `run:` or a github-script `script:`
// is pasted into the script before the shell or Node parses it. Pass the
// value through the step's `env:` and read it from the environment instead.
// The check covers every local action a workflow uses, at any depth.
func TestScriptsHaveNoExpressions(t *testing.T) {
	offenders, err := scanRepo(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offenders {
		t.Error(o)
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
		{"docker action args with expression", `
runs:
  using: docker
  image: Dockerfile
  args:
    - --name
    - ${{ inputs.name }}
`, 1},
		{"docker action entrypoint with expression", `
runs:
  using: docker
  image: Dockerfile
  entrypoint: ${{ inputs.entrypoint }}
`, 1},
		{"docker action without expression", `
runs:
  using: docker
  image: Dockerfile
  entrypoint: /entrypoint.sh
  args:
    - --name
  env:
    NAME: ${{ inputs.name }}
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

// writeRepo writes files (path relative to the repository root -> content)
// into a temporary directory and returns its path.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

const usesNested = `
jobs:
  build:
    steps:
      - uses: ./nested/a/b
`

func TestScanRepo(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"nested local action with expression", map[string]string{
			".github/workflows/ci.yml": usesNested,
			"nested/a/b/action.yml": `
runs:
  using: composite
  steps:
    - shell: bash
      run: echo "${{ inputs.version }}"
`,
		}, []string{"nested/a/b/action.yml: runs step 1"}},
		{"nested local action without expression", map[string]string{
			".github/workflows/ci.yml": usesNested,
			"nested/a/b/action.yaml": `
runs:
  using: composite
  steps:
    - shell: bash
      env:
        VERSION: ${{ inputs.version }}
      run: echo "$VERSION"
`,
		}, nil},
		{"missing local action", map[string]string{
			".github/workflows/ci.yml": usesNested,
		}, []string{".github/workflows/ci.yml: uses ./nested/a/b, but nested/a/b has no action.yml or action.yaml"}},
		{"local action that uses a local action", map[string]string{
			".github/workflows/ci.yml": `
jobs:
  build:
    steps:
      - uses: ./.github/actions/outer
`,
			".github/actions/outer/action.yml": `
runs:
  using: composite
  steps:
    - uses: ./
`,
			"action.yml": `
runs:
  using: docker
  image: Dockerfile
  args:
    - ${{ inputs.name }}
`,
		}, []string{"action.yml: runs.args entry 1"}},
		{"local action outside the repository", map[string]string{
			".github/workflows/ci.yml": `
jobs:
  build:
    steps:
      - uses: ./../other
`,
		}, []string{".github/workflows/ci.yml: uses ./../other, which points outside the repository"}},
		{"reusable workflow", map[string]string{
			".github/workflows/ci.yml": `
jobs:
  call:
    uses: ./.github/workflows/reusable.yml
  missing:
    uses: ./.github/workflows/missing.yml
`,
			".github/workflows/reusable.yml": `
jobs:
  build:
    steps:
      - uses: ./nested/a/b
`,
		}, []string{
			".github/workflows/ci.yml: uses ./.github/workflows/missing.yml, but .github/workflows/missing.yml does not exist",
			".github/workflows/reusable.yml: uses ./nested/a/b, but nested/a/b has no action.yml",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanRepo(writeRepo(t, tt.files))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("scanRepo() = %q, want %d offenders", got, len(tt.want))
			}
			for i, want := range tt.want {
				if !strings.HasPrefix(got[i], want) {
					t.Errorf("scanRepo()[%d] = %q, want prefix %q", i, got[i], want)
				}
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

// TestAURKeyIsPrivate keeps the AUR SSH key from existing with the default
// umask, even for the moment before chmod 600.
func TestAURKeyIsPrivate(t *testing.T) {
	run := stepRun(t, "aur.yml", "Publish to AUR")
	want := `(umask 077; printf '%s\n' "$AUR_SSH_PRIVATE_KEY" > ~/.ssh/aur)`
	if !strings.Contains(run, want) {
		t.Errorf("aur.yml: Publish to AUR does not write the key with %s", want)
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
}
