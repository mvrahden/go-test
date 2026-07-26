package docsync_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DocSyncTestSuite guards the design docs against drifting from the code:
// every exported pkg/gotest API symbol must be mentioned in spec.md, and every
// ```go fence in the design docs must at least parse as Go syntax.
type DocSyncTestSuite struct{}

func readDoc(t *gotest.T, name string) string {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "design", name))
	gotest.NoError(t, err, "reading %s", name)
	return string(data)
}

func (s *DocSyncTestSuite) TestExportedAPIIsDocumented(t *gotest.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "..", "pkg", "gotest"), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	gotest.NoError(t, err)

	var symbols []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() {
					continue
				}
				if fn.Recv == nil {
					symbols = append(symbols, fn.Name.Name)
					continue
				}
				// Methods on the exported T and R types are user-facing API too.
				if recv := receiverName(fn); recv == "T" || recv == "R" {
					symbols = append(symbols, fn.Name.Name)
				}
			}
		}
	}
	gotest.NotEmpty(t, symbols, "expected to find exported symbols in pkg/gotest")

	spec := readDoc(t, "spec.md")
	t.It("mentions every exported function and T/R method in spec.md", func(it *gotest.T) {
		for _, name := range symbols {
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			gotest.Regexp(it, re, spec, "exported symbol %q is not mentioned in docs/design/spec.md", name)
		}
	})
}

func receiverName(fn *ast.FuncDecl) string {
	if len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// fenceRE captures ```go fences and an optional pseudo marker on the preceding line.
var fenceRE = regexp.MustCompile("(?ms)^(<!-- fence:pseudo -->\n)?```go\n(.*?)\n```")

func (s *DocSyncTestSuite) TestGoFencesParse(t *gotest.T) {
	for sub, doc := range gotest.Each(t, []struct {
		Name    string
		Content string
	}{
		{Name: "spec.md", Content: readDoc(t, "spec.md")},
		{Name: "fixtures.md", Content: readDoc(t, "fixtures.md")},
	}) {
		for i, m := range fenceRE.FindAllStringSubmatch(doc.Content, -1) {
			if m[1] != "" {
				continue // marked <!-- fence:pseudo --> — illustrative, not Go
			}
			src := m[2]
			firstLine, _, _ := strings.Cut(src, "\n")
			gotest.True(sub, parsesAsGo(src), "%s: ```go fence #%d starting %q does not parse — fix it or mark it with <!-- fence:pseudo -->", doc.Name, i+1, firstLine)
		}
	}
}

// parsesAsGo accepts declaration fences (wrapped in a package clause) and
// statement fences (additionally wrapped in a function body).
func parsesAsGo(src string) bool {
	fset := token.NewFileSet()
	asDecls := src
	if !strings.HasPrefix(strings.TrimSpace(src), "package ") {
		asDecls = "package p\n\n" + src
	}
	if _, err := parser.ParseFile(fset, "fence.go", asDecls, 0); err == nil {
		return true
	}
	asStmts := "package p\n\nfunc _() {\n" + src + "\n}"
	_, err := parser.ParseFile(fset, "fence.go", asStmts, 0)
	return err == nil
}
