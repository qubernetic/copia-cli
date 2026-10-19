// Package workflows checks the GitHub Actions workflows under .github/workflows
// and the composite actions under .github/actions.
package workflows

import (
	"fmt"
	"os"
	"os/exec"
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

	for _, version := range []string{"", "1.27; echo x", "go1.27", "1", "1.27rc1", "1.27.0.1", "1.27\n", "$(id)"} {
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
}
