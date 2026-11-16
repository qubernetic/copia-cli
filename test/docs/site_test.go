// Package docs_test keeps the documentation site honest: the generated
// command reference links only to its own pages, shows no escaped "<" in
// code, and its navigation lists each page once, and the hand-written pages
// under docs/site/docs link only to command pages that exist (inline links
// and reference definitions) and name only commands that exist.
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

// refDefRE matches a reference-style link definition, "[label]: target",
// indented by at most three spaces, and captures the target without its
// #fragment. A footnote, "[^label]:", is no link.
var refDefRE = regexp.MustCompile(`^ {0,3}\[[^\]^][^\]]*\]:\s*<?([^\s#>]*)`)

// linkTargets returns the targets of a line's inline links and of a
// reference definition on it, without #fragments.
func linkTargets(line string) []string {
	var targets []string
	for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
		targets = append(targets, m[1])
	}
	if m := refDefRE.FindStringSubmatch(line); m != nil {
		targets = append(targets, m[1])
	}
	return targets
}

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
			for _, target := range linkTargets(line) {
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

// lineKind is the Markdown block a line of a generated page belongs to.
type lineKind int

const (
	blankLine      lineKind = iota
	proseLine               // a paragraph, heading, list or definition term
	definitionLine          // a continuation line of a definition (":   ")
	fencedLine              // a ``` fence or a line inside one
	indentedCode            // a line of an indented code block
)

// classifyLines sorts the lines of a generated page into blocks the way
// MkDocs reads them. An indented code block starts after a blank line with
// a line indented by four or more spaces, or by eight or more inside a
// definition, whose own lines printFlags indents by four; it ends at the
// first non-blank line indented less. A definition starts at a line
// beginning ":   " and ends at the first non-blank line indented by less
// than four spaces.
func classifyLines(lines []string) []lineKind {
	kinds := make([]lineKind, len(lines))
	inFence, inDef, inCode, prevBlank := false, false, false, true
	for i, l := range lines {
		t := strings.TrimSpace(l)
		indent := len(l) - len(strings.TrimLeft(l, " "))
		switch {
		case inFence:
			kinds[i] = fencedLine
			inFence = !strings.HasPrefix(t, "```")
			continue
		case t == "":
			kinds[i] = blankLine
			prevBlank = true
			continue
		}
		codeIndent := 4
		if inDef {
			codeIndent = 8
		}
		switch {
		case indent >= codeIndent && (prevBlank || inCode):
			kinds[i], inCode = indentedCode, true
		case indent < 4 && strings.HasPrefix(t, "```"):
			kinds[i], inFence, inCode, inDef = fencedLine, true, false, false
		case strings.HasPrefix(l, ":   "):
			kinds[i], inCode, inDef = proseLine, false, true
		case inDef && indent >= 4:
			kinds[i], inCode = definitionLine, false
		default:
			kinds[i], inCode = proseLine, false
			if indent < 4 {
				inDef = false
			}
		}
		prevBlank = false
	}
	return kinds
}

// escapedInCode returns the numbers, from 1, of the lines of a generated
// page that show "&lt;" literally: in a fenced or indented code block, or
// inside a code span, also one that a line break splits. A span runs from a
// backtick to the next one in the same paragraph or definition. "&lt;" in
// prose and in definitions is what escapeAngles writes for "<" and is
// rendered as "<".
func escapedInCode(lines []string) []int {
	var bad []int
	inSpan := false
	for i, kind := range classifyLines(lines) {
		line := lines[i]
		switch kind {
		case fencedLine, indentedCode:
			inSpan = false
			if strings.Contains(line, "&lt;") {
				bad = append(bad, i+1)
			}
			continue
		case blankLine:
			inSpan = false
			continue
		}
		found := false
		for j, seg := range strings.Split(line, "`") {
			if (j%2 == 1) != inSpan && strings.Contains(seg, "&lt;") {
				found = true
			}
		}
		if found {
			bad = append(bad, i+1)
		}
		if strings.Count(line, "`")%2 == 1 {
			inSpan = !inSpan
		}
	}
	return bad
}

func TestEscapedInCode(t *testing.T) {
	cases := []struct {
		name string
		page string
		want []int
	}{
		{"prose", "Close &lt;number>.", nil},
		{"definition", "`--x`\n:   First &lt;a>\n    then &lt;b>\n\n    and &lt;c>", nil},
		{"paragraph continuation", "Text\n    more &lt;a>", nil},
		{"indented code block", "Text\n\n    run &lt;a>\n\n    more &lt;b>\nText &lt;c>", []int{3, 5}},
		{"indented code block in a definition", "`--x`\n:   Text\n\n        run &lt;a>", []int{4}},
		{"fenced block", "```bash\necho &lt;a>\n```\n&lt;b>", []int{2}},
		{"code span", "Use `--x &lt;a>` with &lt;b>.", []int{1}},
		{"code span split by a line break", "Use `--x\n&lt;a>` with &lt;b>.\n\n&lt;c>", []int{2}},
		{"span ends with its paragraph", "Use `x\n\n&lt;a>", nil},
	}
	for _, c := range cases {
		if got := escapedInCode(strings.Split(c.page, "\n")); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: escapedInCode(%q) = %v, want %v", c.name, c.page, got, c.want)
		}
	}
}

// TestGeneratedPagesShowNoEscapeInCode catches what escapeAngles cannot
// see: it skips ``` fences and code spans within one line only, so "<" in an
// indented code block or in a span a line break splits would be shown as
// "&lt;".
func TestGeneratedPagesShowNoEscapeInCode(t *testing.T) {
	codeLines := 0
	for _, name := range generatedPages(t) {
		data, err := os.ReadFile(filepath.Join(genDir, name))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		for _, n := range escapedInCode(lines) {
			t.Errorf("%s:%d: \"&lt;\" in code, shown literally: %q", name, n, lines[n-1])
		}
		for _, k := range classifyLines(lines) {
			if k == indentedCode {
				codeLines++
			}
		}
	}
	// The pages of alias import and completion hold indented code blocks; a
	// classifier that finds none checks nothing.
	if codeLines == 0 {
		t.Error("found no indented code block in the generated pages")
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

// brokenCommandLinks returns, as "line: target", the links of a hand-written
// page that point at a command page the generator does not write.
func brokenCommandLinks(page sitePage) []string {
	var broken []string
	for i, line := range page.lines {
		for _, target := range linkTargets(line) {
			if target == "" || strings.Contains(target, "://") {
				continue
			}
			resolved := path.Clean(path.Join(path.Dir(page.rel), target))
			if !isGeneratedPath(resolved) {
				continue
			}
			if !generated[path.Base(resolved)] {
				broken = append(broken, fmt.Sprintf("%d: %s", i+1, target))
			}
		}
	}
	return broken
}

func TestBrokenCommandLinks(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"inline link to a missing page", "See [x](../manual/copia-cli_gone.md).", []string{"1: ../manual/copia-cli_gone.md"}},
		{"inline link to a page", "See [x](../manual/copia-cli_issue.md#options).", nil},
		{"reference definition to a missing page", "[x]: ../manual/copia-cli_gone.md", []string{"1: ../manual/copia-cli_gone.md"}},
		{"reference definition with a title", "  [x]: <../manual/copia-cli_gone.md#top> \"Gone\"", []string{"1: ../manual/copia-cli_gone.md"}},
		{"reference definition to a page", "[x]: ../manual/copia-cli_issue.md", nil},
		{"reference definition to another site", "[x]: https://example.com/manual/copia-cli_gone.md", nil},
		{"footnote", "[^x]: ../manual/copia-cli_gone.md", nil},
	}
	for _, c := range cases {
		page := sitePage{rel: "getting-started/page.md", lines: []string{c.line}}
		if got := brokenCommandLinks(page); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: brokenCommandLinks(%q) = %q, want %q", c.name, c.line, got, c.want)
		}
	}
}

func TestSitePagesLinkToRealCommandPages(t *testing.T) {
	for _, page := range handWrittenPages(t) {
		for _, b := range brokenCommandLinks(page) {
			t.Errorf("docs/site/docs/%s:%s, which is not a generated command page", page.rel, b)
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
