// Package docs_test keeps the documentation site honest: the generated
// command reference links only to its own pages, its navigation lists each
// page once, and the hand-written pages under docs/site/docs link only to
// command pages that exist and name only commands that exist.
//
// The tests generate the reference into a temporary directory, so they run
// from a clean checkout where the generated pages are absent and the brand
// submodule is not initialized.
package docs_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/qubernetic/copia-cli/internal/config"
	"github.com/qubernetic/copia-cli/internal/copiacmd"
	"github.com/qubernetic/copia-cli/internal/docs"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// navPrefix is the directory of the command pages inside docs_dir, as
// script/gen-docs.go passes it to docs.WriteNav.
const navPrefix = "manual/"

var (
	// rootCmd is built exactly as script/gen-docs.go builds it.
	rootCmd *cobra.Command
	// genDir holds the generated pages and genNav the navigation file.
	genDir string
	genNav string
	// generated is the set of generated page names, e.g. "copia-cli_issue.md".
	generated map[string]bool
)

// siteDocs is docs/site/docs, relative to this package.
var siteDocs = filepath.Join("..", "..", "docs", "site", "docs")

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "copia-docs-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := func() int {
		defer func() { _ = os.RemoveAll(tmp) }()
		if err := generate(tmp); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}

func generate(tmp string) error {
	f := &cmdutil.Factory{
		IOStreams: iostreams.System(),
		Config: func() (*config.Config, error) {
			return &config.Config{Hosts: map[string]*config.HostConfig{}}, nil
		},
	}
	rootCmd = copiacmd.NewRootCmd(f)
	rootCmd.DisableAutoGenTag = true

	genDir = filepath.Join(tmp, "manual")
	genNav = filepath.Join(tmp, "commands-nav.json")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}
	if err := docs.GenMarkdownTree(rootCmd, genDir); err != nil {
		return fmt.Errorf("generating the manual: %w", err)
	}
	if err := docs.WriteNav(rootCmd, genNav, navPrefix); err != nil {
		return fmt.Errorf("generating the navigation: %w", err)
	}
	entries, err := os.ReadDir(genDir)
	if err != nil {
		return err
	}
	generated = map[string]bool{}
	for _, e := range entries {
		generated[e.Name()] = true
	}
	if len(generated) == 0 {
		return fmt.Errorf("the generator wrote no pages into %s", genDir)
	}
	return nil
}

// linkRE matches an inline Markdown link target, without its #fragment.
var linkRE = regexp.MustCompile(`\]\(\s*<?([^)\s#>]*)[^)]*\)`)

func generatedPages(t *testing.T) []string {
	t.Helper()
	names := make([]string, 0, len(generated))
	for n := range generated {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func TestGeneratedPagesLinkToEachOther(t *testing.T) {
	for _, name := range generatedPages(t) {
		data, err := os.ReadFile(filepath.Join(genDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if !strings.HasSuffix(target, ".md") {
					continue
				}
				if !generated[target] {
					t.Errorf("%s:%d: link to %q, which is not a generated page", name, i+1, target)
				}
			}
		}
	}
}

func TestNavigationHoldsEveryPageOnce(t *testing.T) {
	data, err := os.ReadFile(genNav)
	if err != nil {
		t.Fatal(err)
	}
	var nav any
	if err := json.Unmarshal(data, &nav); err != nil {
		t.Fatalf("navigation is not JSON: %v", err)
	}
	seen := map[string]int{}
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, e := range v {
				walk(e)
			}
		case map[string]any:
			for _, e := range v {
				walk(e)
			}
		case string:
			if !strings.HasPrefix(v, navPrefix) {
				t.Errorf("navigation path %q lacks the prefix %q", v, navPrefix)
			}
			seen[strings.TrimPrefix(v, navPrefix)]++
		default:
			t.Errorf("unexpected navigation entry %#v", v)
		}
	}
	walk(nav)
	for p, n := range seen {
		if n > 1 {
			t.Errorf("navigation lists %s %d times", p, n)
		}
		if !generated[p] {
			t.Errorf("navigation lists %s, which is not a generated page", p)
		}
	}
	for p := range generated {
		if seen[p] == 0 {
			t.Errorf("navigation misses the generated page %s", p)
		}
	}
}

func TestGeneratedPagesHaveNoJekyll(t *testing.T) {
	for _, name := range generatedPages(t) {
		data, err := os.ReadFile(filepath.Join(genDir, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if strings.HasPrefix(text, "---") {
			t.Errorf("%s starts with front matter", name)
		}
		if !strings.HasPrefix(text, "# copia-cli") {
			t.Errorf("%s does not start with \"# copia-cli\"", name)
		}
		for i, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "{%") {
				t.Errorf("%s:%d: Liquid tag {%% in %q", name, i+1, line)
			}
			if strings.Contains(line, "<dl") {
				t.Errorf("%s:%d: HTML definition list in %q", name, i+1, line)
			}
		}
	}
}

// isGeneratedPath reports whether p, relative to docs_dir with slashes, is
// where the generator writes a command page.
func isGeneratedPath(p string) bool {
	dir, file := path.Split(p)
	return dir == navPrefix && strings.HasPrefix(file, "copia-cli") && strings.HasSuffix(file, ".md")
}

type sitePage struct {
	rel   string // relative to docs_dir, with slashes
	lines []string
}

// handWrittenPages returns the pages under docs/site/docs other than the
// generated command pages, which may be present from an earlier run.
func handWrittenPages(t *testing.T) []sitePage {
	t.Helper()
	var pages []sitePage
	err := filepath.WalkDir(siteDocs, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		rel, err := filepath.Rel(siteDocs, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isGeneratedPath(rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		pages = append(pages, sitePage{rel: rel, lines: strings.Split(string(data), "\n")})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatalf("no hand-written pages under %s", siteDocs)
	}
	return pages
}

func TestSitePagesLinkToRealCommandPages(t *testing.T) {
	for _, page := range handWrittenPages(t) {
		for i, line := range page.lines {
			for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if target == "" || strings.Contains(target, "://") {
					continue
				}
				resolved := path.Clean(path.Join(path.Dir(page.rel), target))
				if !isGeneratedPath(resolved) {
					continue
				}
				if !generated[path.Base(resolved)] {
					t.Errorf("docs/site/docs/%s:%d: link to %s, which is not a generated command page", page.rel, i+1, target)
				}
			}
		}
	}
}

// shellLangs are the fence languages whose copia-cli lines are checked.
var shellLangs = map[string]bool{"bash": true, "sh": true, "powershell": true, "console": true, "text": true}

// fenceLang returns the language of an opening fence line, or ok false when
// the line is not a fence.
func fenceLang(line string) (marker, lang string, ok bool) {
	s := strings.TrimSpace(line)
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(s, m) {
			n := len(s) - len(strings.TrimLeft(s, m[:1]))
			info := strings.TrimSpace(s[n:])
			info = strings.TrimLeft(info, "{.")
			if f := strings.Fields(info); len(f) > 0 {
				lang = strings.ToLower(strings.TrimRight(f[0], "}"))
			}
			return s[:n], lang, true
		}
	}
	return "", "", false
}

// commandWords returns the words of a copia-cli command line up to the first
// flag, placeholder, quoted string or variable.
func commandWords(rest string) []string {
	var words []string
	for _, w := range strings.Fields(rest) {
		if strings.ContainsAny(w[:1], "-<\"'$") {
			break
		}
		words = append(words, w)
	}
	return words
}

// namesCommand reports whether a line that starts with "copia-cli " names a
// command. The longest prefix of the words that is a command must hold at
// least one word. Words left over count as positional arguments only when
// that command runs; after a group such as "issue" they name a subcommand
// that does not exist, for example one since renamed or removed.
func namesCommand(line string) bool {
	if line == "copia-cli --help" || line == "copia-cli --version" {
		return true
	}
	words := commandWords(strings.TrimPrefix(line, "copia-cli "))
	if len(words) == 0 {
		return false
	}
	cmd, rest, err := rootCmd.Find(words)
	if err != nil || cmd == nil || cmd == rootCmd {
		return false
	}
	return cmd.Runnable() || len(rest) == 0
}

func TestNamesCommand(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"copia-cli issue nope", false},
		{"copia-cli nope list", false},
		{"copia-cli -R my-org/my-repo", false},
		{"copia-cli issue list", true},
		{"copia-cli issue", true},
		{"copia-cli issue ls --state open", true},
		{"copia-cli repo clone owner/repo", true},
		{"copia-cli --version", true},
		{"copia-cli --help", true},
	}
	for _, c := range cases {
		if got := namesCommand(c.line); got != c.want {
			t.Errorf("namesCommand(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestSitePagesNameRealCommands(t *testing.T) {
	for _, page := range handWrittenPages(t) {
		var open string // the open fence's marker, "" outside a fence
		check := false
		for i, raw := range page.lines {
			if marker, lang, ok := fenceLang(raw); ok {
				if open == "" {
					open, check = marker, shellLangs[lang]
				} else if strings.HasPrefix(marker, open) && lang == "" {
					open, check = "", false
				}
				continue
			}
			if open == "" || !check {
				continue
			}
			line := strings.TrimSpace(raw)
			line = strings.TrimPrefix(line, "$ ")
			if !strings.HasPrefix(line, "copia-cli ") {
				continue
			}
			if !namesCommand(line) {
				t.Errorf("docs/site/docs/%s:%d: %q names no copia-cli command", page.rel, i+1, line)
			}
		}
	}
}

func TestSiteNavNamesExistingPages(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(siteDocs, "..", "mkdocs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Nav any `yaml:"nav"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("mkdocs.yml: %v", err)
	}
	if cfg.Nav == nil {
		t.Fatal("mkdocs.yml has no nav")
	}
	var paths []string
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, e := range v {
				walk(e)
			}
		case map[string]any:
			for _, e := range v {
				walk(e)
			}
		case string:
			if strings.HasSuffix(v, ".md") {
				paths = append(paths, v)
			}
		}
	}
	walk(cfg.Nav)
	if len(paths) == 0 {
		t.Fatal("mkdocs.yml's nav names no page")
	}
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(siteDocs, filepath.FromSlash(p))); err != nil {
			t.Errorf("mkdocs.yml's nav names %s, which is not under docs/site/docs: %v", p, err)
		}
	}
}
