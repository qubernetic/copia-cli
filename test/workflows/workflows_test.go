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

// loadWorkflow parses one file under .github/workflows.
func loadWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var wf workflow
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

// TestCheckReleaseVersion covers the guard aur.yml and copr.yml run on the
// version taken from the release tag before it reaches GITHUB_OUTPUT, PKGBUILD
// and the RPM spec.
func TestCheckReleaseVersion(t *testing.T) {
	script := filepath.Join("..", "..", "script", "check-release-version.sh")

	for _, version := range []string{"1.2.3", "1.2.3-rc.1", "10.0.12-beta2"} {
		t.Run("accepts "+version, func(t *testing.T) {
			out, err := exec.Command(script, version).CombinedOutput()
			if err != nil {
				t.Errorf("check-release-version.sh %q: %v\n%s", version, err, out)
			}
		})
	}

	for _, version := range []string{"", "1.2.3/x", "1.2.3$(id)", "1.2.3'", "1.2.3\n", "1.2.3\nx", "v1.2.3", "1.2", "1.2.3-", "1.2.3-rc 1"} {
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
