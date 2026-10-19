// Package workflows checks the GitHub Actions workflows under .github/workflows.
package workflows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Name string `yaml:"name"`
			Run  string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestRunBlocksHaveNoExpressions keeps values from outside the workflow file
// out of shell source: a `${{ }}` inside `run:` is pasted into the script
// before the shell parses it. Pass the value through the step's `env:` and
// reference it as a quoted shell variable instead.
func TestRunBlocksHaveNoExpressions(t *testing.T) {
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
		var wf workflow
		if err := yaml.Unmarshal(data, &wf); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for jobName, job := range wf.Jobs {
			for i, step := range job.Steps {
				if strings.Contains(step.Run, "${{") {
					t.Errorf("%s: job %q step %d (%q): run block contains ${{; pass the value through env", filepath.Base(file), jobName, i+1, step.Name)
				}
			}
		}
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
