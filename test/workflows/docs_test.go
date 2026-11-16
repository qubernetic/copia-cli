package workflows

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// mainRef is the condition that limits a job or step of docs.yml to main.
const mainRef = "github.ref == 'refs/heads/main'"

// docsStep is the part of a docs.yml step the docs tests read.
type docsStep struct {
	ID              string         `yaml:"id"`
	Name            string         `yaml:"name"`
	If              string         `yaml:"if"`
	Uses            string         `yaml:"uses"`
	Run             string         `yaml:"run"`
	With            map[string]any `yaml:"with"`
	Env             map[string]any `yaml:"env"`
	ContinueOnError any            `yaml:"continue-on-error"`
}

// docsWorkflow is the part of docs.yml the docs tests read.
type docsWorkflow struct {
	On          map[string]any `yaml:"on"`
	Concurrency any            `yaml:"concurrency"`
	Jobs        map[string]struct {
		If              string     `yaml:"if"`
		Needs           any        `yaml:"needs"`
		Env             any        `yaml:"env"`
		ContinueOnError any        `yaml:"continue-on-error"`
		Steps           []docsStep `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadDocsWorkflow(t *testing.T) (docsWorkflow, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "docs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf docsWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("docs.yml: %v", err)
	}
	return wf, string(data)
}

func usesAction(s docsStep, action string) bool {
	return strings.HasPrefix(strings.ToLower(s.Uses), action+"@")
}

// pushOf returns the push trigger of docs.yml as a map.
func pushOf(t *testing.T, wf docsWorkflow) map[string]any {
	t.Helper()
	push, ok := wf.On["push"].(map[string]any)
	if !ok {
		t.Fatalf("docs.yml: on.push is %T, want a mapping", wf.On["push"])
	}
	return push
}

func stringList(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, e := range l {
			out = append(out, fmt.Sprint(e))
		}
	}
	return out
}

// TestDocsNeverRunsOnPullRequests keeps the docs workflow, whose build job
// holds the brand deploy key, off pull requests: it runs on pushes to main
// and on manual dispatch only.
func TestDocsNeverRunsOnPullRequests(t *testing.T) {
	wf, _ := loadDocsWorkflow(t)
	var triggers []string
	for k := range wf.On {
		triggers = append(triggers, k)
	}
	slices.Sort(triggers)
	if strings.Join(triggers, ",") != "push,workflow_dispatch" {
		t.Errorf("docs.yml: triggers %q, want exactly push and workflow_dispatch", triggers)
	}
	if branches := stringList(pushOf(t, wf)["branches"]); strings.Join(branches, ",") != "main" {
		t.Errorf("docs.yml: push.branches %q, want [main]", branches)
	}
}

// TestDocsDeploysFromMainOnly lets a dispatch from another branch rehearse the
// build without publishing it: the Pages upload and the deploy job run on main
// only, the rehearsal upload everywhere else.
func TestDocsDeploysFromMainOnly(t *testing.T) {
	wf, _ := loadDocsWorkflow(t)
	deploy, ok := wf.Jobs["deploy"]
	if !ok {
		t.Fatal("docs.yml: no deploy job")
	}
	if deploy.If != mainRef {
		t.Errorf("docs.yml: deploy job if %q, want %q", deploy.If, mainRef)
	}
	pagesUploads, rehearsals := 0, 0
	for _, name := range []string{"build", "deploy"} {
		for i, s := range wf.Jobs[name].Steps {
			switch {
			case usesAction(s, "actions/upload-pages-artifact"):
				pagesUploads++
				if name != "build" || s.If != mainRef {
					t.Errorf("docs.yml: job %q step %d uploads the Pages artifact with if %q, want job build and %q", name, i+1, s.If, mainRef)
				}
			case usesAction(s, "actions/upload-artifact"):
				rehearsals++
				if want := "github.ref != 'refs/heads/main'"; s.If != want {
					t.Errorf("docs.yml: job %q step %d uploads the rehearsal build with if %q, want %q", name, i+1, s.If, want)
				}
			case usesAction(s, "actions/deploy-pages") && name != "deploy":
				t.Errorf("docs.yml: job %q step %d deploys Pages outside the deploy job", name, i+1)
			}
		}
	}
	if pagesUploads != 1 || rehearsals != 1 {
		t.Errorf("docs.yml: %d Pages uploads and %d rehearsal uploads, want 1 each", pagesUploads, rehearsals)
	}
	for name, job := range wf.Jobs {
		if name == "build" || name == "deploy" {
			continue
		}
		for i, s := range job.Steps {
			if usesAction(s, "actions/upload-pages-artifact") || usesAction(s, "actions/deploy-pages") {
				t.Errorf("docs.yml: job %q step %d uses %s", name, i+1, s.Uses)
			}
		}
	}
}

// secretsRE matches the secrets context as a word, so secrets.X,
// secrets['X'] and toJSON(secrets) all count.
var secretsRE = regexp.MustCompile(`\bsecrets\b`)

// withoutComments drops the comment lines of a workflow, which run nothing.
func withoutComments(raw string) string {
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// TestDocsBrandKeyStaysInOneStep keeps the brand repository's deploy key in
// the one checkout that fetches the theme: actions/checkout fetches the main
// repository with its ssh-key too, so the first checkout carries no key and
// no submodules, and neither checkout leaves credentials behind.
func TestDocsBrandKeyStaysInOneStep(t *testing.T) {
	wf, raw := loadDocsWorkflow(t)
	if n := len(secretsRE.FindAllString(withoutComments(raw), -1)); n != 1 {
		t.Errorf("docs.yml names the secrets context %d times, want once", n)
	}
	for name, job := range wf.Jobs {
		if secretsRE.MatchString(fmt.Sprint(job.Env)) {
			t.Errorf("docs.yml: job %q env references a secret", name)
		}
	}
	steps := wf.Jobs["build"].Steps
	var keyed []int
	for i, s := range steps {
		if secretsRE.MatchString(fmt.Sprint(s.With, s.Env, s.Run, s.If)) {
			keyed = append(keyed, i)
		}
	}
	if len(keyed) != 1 {
		t.Fatalf("docs.yml: build steps %v reference the secrets context, want exactly one", keyed)
	}
	brand := steps[keyed[0]]
	if !usesAction(brand, "actions/checkout") {
		t.Errorf("docs.yml: the step with the secret uses %q, want actions/checkout", brand.Uses)
	}
	for key, want := range map[string]string{
		"repository":          "qubernetic/brand",
		"path":                "docs/site/brand",
		"persist-credentials": "false",
		"ref":                 "${{ steps.brand.outputs.sha }}",
		"ssh-key":             "${{ secrets.BRAND_DEPLOY_KEY }}",
	} {
		if got := fmt.Sprint(brand.With[key]); got != want {
			t.Errorf("docs.yml: brand checkout with.%s %q, want %q", key, got, want)
		}
	}
	pin := slices.IndexFunc(steps, func(s docsStep) bool { return s.ID == "brand" })
	if pin < 0 || pin > keyed[0] {
		t.Errorf("docs.yml: no step with id brand before the brand checkout")
	} else if !strings.Contains(steps[pin].Run, "git rev-parse HEAD:docs/site/brand") || !strings.Contains(steps[pin].Run, `>> "$GITHUB_OUTPUT"`) {
		t.Errorf("docs.yml: step brand does not write git rev-parse HEAD:docs/site/brand to GITHUB_OUTPUT")
	}
	first := slices.IndexFunc(steps, func(s docsStep) bool { return usesAction(s, "actions/checkout") })
	if first < 0 || first == keyed[0] {
		t.Fatal("docs.yml: build has no checkout of its own before the brand checkout")
	}
	own := steps[first]
	if fmt.Sprint(own.With["persist-credentials"]) != "false" {
		t.Error("docs.yml: the first checkout does not set persist-credentials: false")
	}
	for _, key := range []string{"ssh-key", "submodules"} {
		if _, ok := own.With[key]; ok {
			t.Errorf("docs.yml: the first checkout sets %s", key)
		}
	}
	checkouts := 0
	for _, s := range steps {
		if usesAction(s, "actions/checkout") {
			checkouts++
		}
	}
	if checkouts != 2 {
		t.Errorf("docs.yml: build has %d checkouts, want 2", checkouts)
	}
}

// TestDocsBuildIsStrict fails the build on any MkDocs warning, a broken link
// or a page missing from the navigation among them. The build step runs that
// one command, so nothing such as "|| true" swallows its exit code, and
// neither the step nor the job may continue on error.
func TestDocsBuildIsStrict(t *testing.T) {
	wf, _ := loadDocsWorkflow(t)
	build := wf.Jobs["build"]
	if build.ContinueOnError != nil {
		t.Errorf("docs.yml: job build sets continue-on-error: %v", build.ContinueOnError)
	}
	found := 0
	for i, s := range build.Steps {
		if s.ContinueOnError != nil {
			t.Errorf("docs.yml: build step %d sets continue-on-error: %v", i+1, s.ContinueOnError)
		}
		if !strings.Contains(s.Run, "mkdocs build --strict") {
			continue
		}
		found++
		if run := strings.TrimSpace(s.Run); strings.ContainsAny(run, "|;&\n") {
			t.Errorf("docs.yml: build step %d runs %q, want the mkdocs build alone, without |, ;, & or a second line", i+1, run)
		}
	}
	if found != 1 {
		t.Errorf("docs.yml: %d run blocks contain mkdocs build --strict, want 1", found)
	}
}

// TestDocsRequirementsPinTheThemesMaterial keeps the docs build on the
// mkdocs-material the Qubernetic theme is built for, installed from a
// requirements file pinned by hash like copr-cli's.
func TestDocsRequirementsPinTheThemesMaterial(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "requirements.in"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(in)); got != "mkdocs-material==9.7.6" {
		t.Errorf("docs/site/requirements.in is %q, want exactly mkdocs-material==9.7.6", got)
	}
	const file = "docs/site/requirements.txt"
	var installs []string
	for _, line := range pipInstalls(t, "docs.yml") {
		got, problem := requirementsFile(line)
		if problem != "" {
			t.Errorf("docs.yml: %s", problem)
		}
		installs = append(installs, got)
	}
	if strings.Join(installs, ",") != file {
		t.Errorf("docs.yml installs from %q, want exactly %s", installs, file)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mkdocs-material==9.7.6 ") {
		t.Errorf("%s does not pin mkdocs-material==9.7.6", file)
	}
	for _, req := range unhashedRequirements(string(data)) {
		t.Errorf("%s: %s is not pinned with == and --hash=sha256:", file, req)
	}
}

// TestDocsPathsCoverTheGenerator rebuilds the site whenever anything the
// manual is generated from, or the brand pin, changes.
func TestDocsPathsCoverTheGenerator(t *testing.T) {
	wf, _ := loadDocsWorkflow(t)
	paths := stringList(pushOf(t, wf)["paths"])
	for _, want := range []string{
		"docs/site/**",
		"pkg/cmd/**",
		"internal/docs/**",
		"internal/copiacmd/**",
		"pkg/cmdutil/**",
		"go.mod",
		"go.sum",
		"script/gen-docs.go",
		".gitmodules",
		".github/workflows/docs.yml",
	} {
		if !slices.Contains(paths, want) {
			t.Errorf("docs.yml: push.paths has no %q", want)
		}
	}
}

// TestDocsRehearsalCannotCancelADeploy gives each ref its own concurrency
// group: with cancel-in-progress, a rehearsal dispatched from a branch would
// otherwise cancel a main run that is building or deploying the live site.
func TestDocsRehearsalCannotCancelADeploy(t *testing.T) {
	wf, _ := loadDocsWorkflow(t)
	var group string
	switch c := wf.Concurrency.(type) {
	case string:
		group = c
	case map[string]any:
		group = fmt.Sprint(c["group"])
	}
	if want := "pages-${{ github.ref }}"; group != want {
		t.Errorf("docs.yml: concurrency group %q, want %q, one group per ref", group, want)
	}
}

// TestBrandSubmoduleSkipsRecursiveClones keeps `git clone --recursive` and a
// bare `git submodule update --init` working for anyone without access to the
// private theme: the submodule is fetched over HTTPS, so SSH is not required
// of those who do have access, and `update = none` skips it unless a build
// asks for it with `git submodule update --init --checkout docs/site/brand`.
func TestBrandSubmoduleSkipsRecursiveClones(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".gitmodules"))
	if err != nil {
		t.Fatal(err)
	}
	section := ""
	got := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if section != `[submodule "docs/site/brand"]` {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			got[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	want := map[string]string{
		"path":   "docs/site/brand",
		"url":    "https://github.com/qubernetic/brand.git",
		"update": "none",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf(".gitmodules: docs/site/brand %s = %q, want %q", key, got[key], value)
		}
	}
}

// TestDocsInstallNeedsNoApt installs MkDocs Material with the pip3 the
// ubuntu-24.04 runner already has: an apt-get update would make the deploy on
// main depend on the apt mirrors as well.
func TestDocsInstallNeedsNoApt(t *testing.T) {
	wf, _ := loadDocsWorkflow(t)
	for name, job := range wf.Jobs {
		for i, s := range job.Steps {
			if strings.Contains(s.Run, "apt-get") || strings.Contains(s.Run, "apt install") {
				t.Errorf("docs.yml: job %q step %d runs apt: %q", name, i+1, strings.TrimSpace(s.Run))
			}
		}
	}
}
