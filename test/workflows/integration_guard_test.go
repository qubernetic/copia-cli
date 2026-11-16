package workflows

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// integrationHelpers is the single reviewed exemption of the guard below: its
// assertion helpers fail a test with their own message, which names the
// expected text and the length of the output in bytes, never the output.
const integrationHelpers = "test/integration/helpers_test.go"

// versionTest may log versionLog once: the Copia version step of the
// integration job prints it on purpose, and it names no account data.
const (
	versionTest = "TestAPI_Version"
	versionLog  = `t.Logf("Copia version (GET /version): %s", v.Version)`
)

// testifyAllowed maps the testify assertions the integration tests may call
// outside integrationHelpers to the index of their first message argument.
// None of them prints the compared output in full.
var testifyAllowed = map[string]int{
	"NoError":       2,
	"Error":         2,
	"ErrorContains": 3,
	"Greater":       3,
	"True":          2,
	"False":         2,
}

// testFailures are the testing.T methods that print their arguments.
var testFailures = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true}

// TestIntegrationTestsPrintNoAccountData keeps the test account's data (its
// login, private repositories, notifications and API responses) out of the
// public CI log: go test prints a failing test's log lines and testify prints
// the output it compares, so every file under test/integration asserts on
// output only through the helpers of integrationHelpers. go/parser ignores
// the integration build tag, so the guard runs without it and without secrets.
func TestIntegrationTestsPrintNoAccountData(t *testing.T) {
	root := filepath.Join("..", "..")
	helpers := false
	err := filepath.WalkDir(filepath.Join(root, "test", "integration"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		helpers = helpers || rel == integrationHelpers
		leaks, err := integrationLeaks(rel, src)
		if err != nil {
			return err
		}
		for _, l := range leaks {
			t.Errorf("%s prints test account data on failure; assert on output through the helpers of %s", l, integrationHelpers)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !helpers {
		t.Errorf("%s not found", integrationHelpers)
	}
}

// integrationLeaks parses one Go file of test/integration and reports, as
// file:line:column, every call that can print command output or a response
// body when a test fails: a Log or Logf call other than versionLog in
// versionTest, and outside integrationHelpers a testing.T Error, Errorf, Fatal
// or Fatalf call with arguments, a testify assertion missing from
// testifyAllowed, an allowed one whose message arguments are not all string
// literals, and a dot import of testify.
func integrationLeaks(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var leaks []string
	report := func(n ast.Node, what string) {
		leaks = append(leaks, fset.Position(n.Pos()).String()+": "+what)
	}
	packages := map[string]bool{}
	testify := map[string]bool{}
	for _, imp := range file.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		pkg := path.Base(p)
		if imp.Name != nil {
			pkg = imp.Name.Name
		}
		packages[pkg] = true
		if p == "github.com/stretchr/testify/assert" || p == "github.com/stretchr/testify/require" {
			if pkg == "." {
				report(imp, "dot import of "+p)
			}
			testify[pkg] = true
		}
	}
	helpers := name == integrationHelpers
	for _, decl := range file.Decls {
		fn, _ := decl.(*ast.FuncDecl)
		versionLogged := false
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			what := x.Name + "." + sel.Sel.Name
			switch {
			case testify[x.Name]:
				if helpers {
					return true
				}
				first, ok := testifyAllowed[sel.Sel.Name]
				if !ok {
					report(call, what)
					return true
				}
				for _, arg := range call.Args[min(first, len(call.Args)):] {
					if lit, ok := arg.(*ast.BasicLit); !ok || lit.Kind != token.STRING {
						report(call, what+" message")
						break
					}
				}
			case packages[x.Name]:
				// A package function such as log.Printf, not a test method.
			case sel.Sel.Name == "Log" || sel.Sel.Name == "Logf":
				if fn != nil && fn.Name.Name == versionTest && !versionLogged && render(fset, call) == versionLog {
					versionLogged = true
					return true
				}
				report(call, what)
			case testFailures[sel.Sel.Name] && len(call.Args) > 0 && !helpers:
				report(call, what)
			}
			return true
		})
	}
	return leaks, nil
}

// render prints an expression as gofmt would.
func render(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, n); err != nil {
		return ""
	}
	return b.String()
}

func TestIntegrationLeaks(t *testing.T) {
	const file = "test/integration/x_test.go"
	tests := []struct {
		name string
		path string
		body string
		want []string
	}{
		{"t.Logf of output", file, `func TestX(t *testing.T) { t.Logf("%s", out) }`, []string{file + ":8:28: t.Logf"}},
		{"t.Log", file, `func TestX(t *testing.T) { t.Log(out) }`, []string{file + ":8:28: t.Log"}},
		{"Logf on another receiver", file, `func TestX(t *testing.T) { t.Run("a", func(tt *testing.T) { tt.Logf("x") }) }`, []string{file + ":8:61: tt.Logf"}},
		{"assert.Contains", file, `func TestX(t *testing.T) { assert.Contains(t, out, "x") }`, []string{file + ":8:28: assert.Contains"}},
		{"require.NotEmpty", file, `func TestX(t *testing.T) { require.NotEmpty(t, out) }`, []string{file + ":8:28: require.NotEmpty"}},
		{"assert.New", file, `func TestX(t *testing.T) { a := assert.New(t); _ = a }`, []string{file + ":8:33: assert.New"}},
		{"formatted message", file, `func TestX(t *testing.T) { require.True(t, ok, "%q", body) }`, []string{file + ":8:28: require.True message"}},
		{"NoError message", file, `func TestX(t *testing.T) { require.NoError(t, err, "body: %s", body) }`, []string{file + ":8:28: require.NoError message"}},
		{"ErrorContains message", file, `func TestX(t *testing.T) { assert.ErrorContains(t, err, "404", out) }`, []string{file + ":8:28: assert.ErrorContains message"}},
		{"t.Errorf", file, `func TestX(t *testing.T) { t.Errorf("%s", out) }`, []string{file + ":8:28: t.Errorf"}},
		{"t.Error", file, `func TestX(t *testing.T) { t.Error(out) }`, []string{file + ":8:28: t.Error"}},
		{"t.Fatal", file, `func TestX(t *testing.T) { t.Fatal(out) }`, []string{file + ":8:28: t.Fatal"}},
		{"t.Fatalf", file, `func TestX(t *testing.T) { t.Fatalf("%s", out) }`, []string{file + ":8:28: t.Fatalf"}},
		{"clean file", file, `func TestX(t *testing.T) {
	require.NoError(t, err)
	require.Error(t, err)
	assert.ErrorContains(t, err, "404")
	require.Greater(t, n, int64(0), "failed to parse issue number")
	require.True(t, ok, "no blank line after the headers")
	assert.False(t, ok)
	assertOutputContains(t, out, "x")
	t.Skipf("server returned error: %v", err)
}`, nil},
		{"version line", "test/integration/api_test.go", `func TestAPI_Version(t *testing.T) { t.Logf("Copia version (GET /version): %s", v.Version) }`, nil},
		{"second log in TestAPI_Version", "test/integration/api_test.go", `func TestAPI_Version(t *testing.T) {
	t.Logf("Copia version (GET /version): %s", v.Version)
	t.Logf("Copia version (GET /version): %s", v.Version)
}`, []string{"test/integration/api_test.go:10:2: t.Logf"}},
		{"other log in TestAPI_Version", "test/integration/api_test.go", `func TestAPI_Version(t *testing.T) { t.Logf("body: %s", body) }`, []string{"test/integration/api_test.go:8:38: t.Logf"}},
		{"version line in another test", file, `func TestY(t *testing.T) { t.Logf("Copia version (GET /version): %s", v.Version) }`, []string{file + ":8:28: t.Logf"}},
		{"helpers may assert", integrationHelpers, `func assertX(t *testing.T) { t.Errorf("%d bytes", len(out)); assert.Contains(t, out, "x") }`, nil},
		{"helpers may not log", integrationHelpers, `func assertX(t *testing.T) { t.Logf("%s", out) }`, []string{integrationHelpers + ":8:30: t.Logf"}},
		{"aliased testify", file, `func TestX(t *testing.T) { tassert.Contains(t, out, "x") }`, []string{file + ":8:28: tassert.Contains"}},
		{"log package is not t.Log", file, `func TestX(t *testing.T) { log.Printf("x") }`, nil},
		{"error method is not t.Error", file, `func TestX(t *testing.T) { _ = err.Error() }`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imports := "import (\n\t\"github.com/stretchr/testify/assert\"\n\t\"github.com/stretchr/testify/require\"\n)\n"
			if strings.Contains(tt.body, "tassert.") {
				imports = "import (\n\ttassert \"github.com/stretchr/testify/assert\"\n\n)\n"
			}
			src := "package integration\n\n" + imports + "\n" + tt.body + "\n"
			got, err := integrationLeaks(tt.path, []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIntegrationLeaks_DotImport(t *testing.T) {
	src := "package integration\n\nimport . \"github.com/stretchr/testify/assert\"\n\nfunc TestX(t *testing.T) { Contains(t, out, \"x\") }\n"
	got, err := integrationLeaks("test/integration/x_test.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := "test/integration/x_test.go:3:8: dot import of github.com/stretchr/testify/assert"
	if strings.Join(got, "\n") != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIntegrationLeaks_BuildTag(t *testing.T) {
	src := "//go:build integration\n\npackage integration\n\nfunc TestX(t *testing.T) { t.Logf(\"%s\", out) }\n"
	got, err := integrationLeaks("test/integration/x_test.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("got %q, want one leak: the build tag must not hide the file", got)
	}
}

func TestIntegrationLeaks_ParseError(t *testing.T) {
	if _, err := integrationLeaks("test/integration/x_test.go", []byte("package")); err == nil {
		t.Error("want a parse error")
	}
}
