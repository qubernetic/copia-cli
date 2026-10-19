// Package workflows checks the GitHub Actions workflows under .github/workflows,
// the composite actions under .github/actions and every local action they use.
package workflows

import (
	"crypto/sha256"
	"encoding/base64"
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
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
	With struct {
		Script             string `yaml:"script"`
		PersistCredentials *bool  `yaml:"persist-credentials"`
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

// pipInstalls returns every line of a workflow that runs pip install, trimmed.
func pipInstalls(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", file))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "pip install") || strings.Contains(line, "pip3 install") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

// requirementsFile returns the file a `pip3 install --require-hashes -r
// <file>` line installs, or a message when the line names a package or does
// not require hashes.
func requirementsFile(line string) (string, string) {
	fields := strings.Fields(line)
	i := 0
	for i < len(fields) && fields[i] != "install" {
		i++
	}
	var file string
	hashes := false
	for i++; i < len(fields); i++ {
		switch {
		case fields[i] == "--require-hashes":
			hashes = true
		case fields[i] == "-r" && i+1 < len(fields):
			i++
			file = fields[i]
		case strings.HasPrefix(fields[i], "-"):
		default:
			return "", fmt.Sprintf("%q names the package %s; install it from a requirements file with hashes", line, fields[i])
		}
	}
	if file == "" || !hashes {
		return "", fmt.Sprintf("%q does not install with --require-hashes -r <file>", line)
	}
	return file, ""
}

// unhashedRequirements returns every requirement of a pip requirements file
// that is not pinned with == or carries no --hash=sha256: entry.
func unhashedRequirements(data string) []string {
	var bad []string
	for _, req := range strings.Split(strings.ReplaceAll(data, "\\\n", " "), "\n") {
		if i := strings.Index(req, "#"); i >= 0 && !strings.Contains(req[:i], "--hash") {
			req = req[:i]
		}
		req = strings.TrimSpace(req)
		if req == "" {
			continue
		}
		if !strings.Contains(strings.Fields(req)[0], "==") || !strings.Contains(req, "--hash=sha256:") {
			bad = append(bad, strings.Fields(req)[0])
		}
	}
	return bad
}

// coprRequirements returns the requirements file copr.yml installs copr-cli
// from, relative to the repository root.
func coprRequirements(t *testing.T) string {
	t.Helper()
	lines := pipInstalls(t, "copr.yml")
	if len(lines) != 1 {
		t.Fatalf("copr.yml: %d pip install lines, want 1: %q", len(lines), lines)
	}
	file, problem := requirementsFile(lines[0])
	if problem != "" {
		t.Fatalf("copr.yml: %s", problem)
	}
	return file
}

// TestCoprCLIIsPinnedByHash keeps copr.yml from installing a package PyPI can
// change under it: copr-cli and its dependencies run in the job that holds
// COPR_TOKEN, so each one is pinned to a version and its sha256 hashes.
func TestCoprCLIIsPinnedByHash(t *testing.T) {
	file := coprRequirements(t)
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "copr-cli==") {
		t.Errorf("%s does not pin copr-cli", file)
	}
	for _, req := range unhashedRequirements(string(data)) {
		t.Errorf("%s: %s is not pinned with == and --hash=sha256:", file, req)
	}
}

func TestRequirementsFile(t *testing.T) {
	for _, tc := range []struct{ line, file string }{
		{"pip3 install --require-hashes -r rpmbuild/requirements.txt", "rpmbuild/requirements.txt"},
		{"pip3 install --require-hashes --no-build-isolation -r a.txt", "a.txt"},
		{"pip install -r a.txt --require-hashes", "a.txt"},
	} {
		if file, problem := requirementsFile(tc.line); file != tc.file || problem != "" {
			t.Errorf("requirementsFile(%q) = %q, %q, want %q", tc.line, file, problem, tc.file)
		}
	}
	for _, line := range []string{
		"pip3 install copr-cli",
		"pip3 install -r a.txt",
		"pip3 install --require-hashes -r a.txt copr-cli",
		"pip3 install --require-hashes",
	} {
		if file, problem := requirementsFile(line); problem == "" {
			t.Errorf("requirementsFile(%q) = %q, want a problem", line, file)
		}
	}
}

func TestUnhashedRequirements(t *testing.T) {
	data := `# comment
a==1.0 \
    --hash=sha256:00 \
    --hash=sha256:11
    # via b
b==2.0 --hash=sha256:22  # via c
c>=1.0 --hash=sha256:33
d==1.0
e
`
	got := unhashedRequirements(data)
	want := []string{"c>=1.0", "d==1.0", "e"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("unhashedRequirements() = %q, want %q", got, want)
	}
}

// isCheckout reports whether a step uses actions/checkout.
func isCheckout(s step) bool {
	return strings.HasPrefix(strings.ToLower(s.Uses), "actions/checkout@")
}

// TestVersionGateRunsFirst keeps every step that installs or configures
// anything in the release jobs behind the version gate: the step with id
// version comes right after actions/checkout.
func TestVersionGateRunsFirst(t *testing.T) {
	for _, file := range []string{"aur.yml", "copr.yml"} {
		for name, job := range loadWorkflow(t, file).Jobs {
			checkout := -1
			for i, s := range job.Steps {
				if isCheckout(s) {
					checkout = i
					break
				}
			}
			if checkout < 0 {
				t.Errorf("%s: job %q has no actions/checkout step", file, name)
				continue
			}
			if next := checkout + 1; next >= len(job.Steps) || job.Steps[next].ID != "version" {
				t.Errorf("%s: job %q: the step after actions/checkout is not the one with id version", file, name)
			}
		}
	}
}

// TestReleaseCheckoutsDropCredentials keeps the GITHUB_TOKEN out of the git
// credential store in the release jobs: neither runs authenticated git against
// GitHub, and gh reads its own GH_TOKEN.
func TestReleaseCheckoutsDropCredentials(t *testing.T) {
	for _, file := range []string{"aur.yml", "copr.yml"} {
		for name, job := range loadWorkflow(t, file).Jobs {
			for i, s := range job.Steps {
				if !isCheckout(s) {
					continue
				}
				if p := s.With.PersistCredentials; p == nil || *p {
					t.Errorf("%s: job %q step %d: actions/checkout does not set persist-credentials: false", file, name, i+1)
				}
			}
		}
	}
}

// TestDependabotUpdatesCoprRequirements keeps the hash-pinned copr-cli
// requirements from going stale: Dependabot watches their directory.
func TestDependabotUpdatesCoprRequirements(t *testing.T) {
	dir := "/" + path.Dir(coprRequirements(t))
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
			Directory string `yaml:"directory"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, u := range cfg.Updates {
		if u.Ecosystem == "pip" && path.Clean(u.Directory) == dir {
			return
		}
	}
	t.Errorf("dependabot.yml has no pip entry for %s", dir)
}

// TestCoprInstallRunsInCI keeps the hash-pinned copr-cli install exercised on
// every pull request: ci.yml runs the same Install copr-cli step as copr.yml,
// which only runs on a release.
func TestCoprInstallRunsInCI(t *testing.T) {
	ci, copr := stepRun(t, "ci.yml", "Install copr-cli"), stepRun(t, "copr.yml", "Install copr-cli")
	if ci != copr {
		t.Errorf("ci.yml Install copr-cli run block differs from copr.yml:\n%s\nwant:\n%s", ci, copr)
	}
}

// TestCoprInstallHasNoSecrets keeps the copr-install job of ci.yml, which runs
// PyPI code on pull requests, away from every secret and from the GITHUB_TOKEN
// in the git credential store.
func TestCoprInstallHasNoSecrets(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Env   any `yaml:"env"`
			Steps []struct {
				Uses string `yaml:"uses"`
				Run  any    `yaml:"run"`
				With any    `yaml:"with"`
				Env  any    `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["copr-install"]
	if !ok {
		t.Fatal("ci.yml has no copr-install job")
	}
	if strings.Contains(fmt.Sprint(job.Env), "secrets.") {
		t.Error("ci.yml: job copr-install env mentions secrets.")
	}
	for i, s := range job.Steps {
		for field, v := range map[string]any{"run": s.Run, "with": s.With, "env": s.Env} {
			if strings.Contains(fmt.Sprint(v), "secrets.") {
				t.Errorf("ci.yml: job copr-install step %d: %s mentions secrets.", i+1, field)
			}
		}
	}
	for i, s := range loadWorkflow(t, "ci.yml").Jobs["copr-install"].Steps {
		if p := s.With.PersistCredentials; isCheckout(s) && (p == nil || *p) {
			t.Errorf("ci.yml: job copr-install step %d: actions/checkout does not set persist-credentials: false", i+1)
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
