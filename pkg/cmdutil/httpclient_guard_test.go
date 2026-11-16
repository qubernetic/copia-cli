package cmdutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guardAllowedFile is the only file that may build an http.Client itself.
const guardAllowedFile = "pkg/cmdutil/httpclient.go"

// TestNoUnguardedHTTPClients fails when production code under pkg/, internal/
// or cmd/ builds an HTTP client without the same-origin redirect policy of
// NewHTTPClient.
func TestNoUnguardedHTTPClients(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, dir := range []string{"pkg", "internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			violations, err := unguardedHTTPClients(filepath.ToSlash(rel), src)
			if err != nil {
				return err
			}
			for _, v := range violations {
				t.Errorf("%s; use cmdutil.NewHTTPClient()", v)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// unguardedHTTPClients parses one Go file and reports every place that builds
// an http.Client (a composite literal or new(http.Client)) outside
// guardAllowedFile, or uses http.DefaultClient or one of the package-level
// helpers that send through it.
func unguardedHTTPClients(path string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	pkgName := ""
	for _, imp := range file.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == "net/http" {
			pkgName = "http"
			if imp.Name != nil {
				pkgName = imp.Name.Name
			}
		}
	}
	if pkgName == "" || pkgName == "_" {
		return nil, nil
	}
	isHTTP := func(e ast.Expr, names ...string) (string, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != pkgName {
			return "", false
		}
		for _, n := range names {
			if sel.Sel.Name == n {
				return n, true
			}
		}
		return "", false
	}
	var violations []string
	report := func(n ast.Node, what string) {
		violations = append(violations, fset.Position(n.Pos()).String()+": "+what)
	}
	allowLiteral := path == guardAllowedFile
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if _, ok := isHTTP(n.Type, "Client"); ok && !allowLiteral {
				report(n, "http.Client literal")
			}
		case *ast.CallExpr:
			if fn, ok := n.Fun.(*ast.Ident); ok && fn.Name == "new" && len(n.Args) == 1 {
				if _, ok := isHTTP(n.Args[0], "Client"); ok && !allowLiteral {
					report(n, "new(http.Client)")
				}
			}
		case *ast.SelectorExpr:
			if name, ok := isHTTP(n, "DefaultClient", "Get", "Head", "Post", "PostForm"); ok {
				report(n, "http."+name)
			}
		}
		return true
	})
	return violations, nil
}

func TestUnguardedHTTPClients_Forms(t *testing.T) {
	const otherFile = "pkg/cmd/x/x.go"
	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{"pointer literal", otherFile, `var c = &http.Client{}`, 1},
		{"literal with timeout", otherFile, `var c = &http.Client{Timeout: time.Second}`, 1},
		{"value literal", otherFile, `var c = http.Client{}`, 1},
		{"new", otherFile, `var c = new(http.Client)`, 1},
		{"DefaultClient", otherFile, `var c = http.DefaultClient`, 1},
		{"Get", otherFile, `func f() { _, _ = http.Get("u") }`, 1},
		{"Head", otherFile, `func f() { _, _ = http.Head("u") }`, 1},
		{"Post", otherFile, `func f() { _, _ = http.Post("u", "", nil) }`, 1},
		{"PostForm", otherFile, `func f() { _, _ = http.PostForm("u", nil) }`, 1},
		{"method value", otherFile, `var f = http.Get`, 1},
		{"aliased import", otherFile, `var c = &nethttp.Client{}`, 1},
		{"pointer type is fine", otherFile, `var c *http.Client`, 0},
		{"NewHTTPClient is fine", otherFile, `var c = cmdutil.NewHTTPClient()`, 0},
		{"client method is fine", otherFile, `func f(c *http.Client) { _, _ = c.Get("u") }`, 0},
		{"other package Get is fine", otherFile, `func f() { _ = os.Getenv("u") }`, 0},
		{"httpclient.go literal allowed", guardAllowedFile, `var c = &http.Client{CheckRedirect: nil}`, 0},
		{"httpclient.go new allowed", guardAllowedFile, `var c = new(http.Client)`, 0},
		{"httpclient.go DefaultClient refused", guardAllowedFile, `var c = http.DefaultClient`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imp := `"net/http"`
			if strings.Contains(tt.body, "nethttp.") {
				imp = `nethttp "net/http"`
			}
			src := "package x\n\nimport " + imp + "\n\n" + tt.body + "\n"
			got, err := unguardedHTTPClients(tt.path, []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d violations %q, want %d", len(got), got, tt.want)
			}
		})
	}
}

func TestUnguardedHTTPClients_NoNetHTTPImport(t *testing.T) {
	src := "package x\n\nimport http \"example.com/fake\"\n\nvar c = &http.Client{}\n"
	got, err := unguardedHTTPClients("pkg/x.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %q, want none: the file does not import net/http", got)
	}
}

func TestUnguardedHTTPClients_ParseError(t *testing.T) {
	if _, err := unguardedHTTPClients("pkg/x.go", []byte("package")); err == nil {
		t.Error("want a parse error")
	}
}
