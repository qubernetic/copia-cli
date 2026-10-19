// Package workflows checks the GitHub Actions workflows under .github/workflows.
package workflows

import (
	"os"
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

