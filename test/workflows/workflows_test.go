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
