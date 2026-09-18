package conventions_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SequentialReasonTestSuite holds the repository's own suites to the rule in
// ARCHITECTURE.md: a sequential suite with more than one test method says why.
type SequentialReasonTestSuite struct{}

// suiteShape is what the rule needs to know about one suite type.
type suiteShape struct {
	file      string
	tests     int
	parallel  bool
	exclusive bool
	reason    bool
}

// scanRoots are the trees that hold the repository's suites; testdata is skipped.
var scanRoots = []string{"cmd", "internal", "pkg", "tests", "examples"}

// scanSuites parses every Go file under the roots and keys suites by
// directory, package and type, so a ptest and a pxtest suite stay apart.
func scanSuites(t *gotest.T, root string) map[string]*suiteShape {
	suites := map[string]*suiteShape{}
	shape := func(key string) *suiteShape {
		if suites[key] == nil {
			suites[key] = &suiteShape{}
		}
		return suites[key]
	}
	for _, dir := range scanRoots {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), "testdata") || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasPrefix(d.Name(), "ƒƒ_") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			prefix := filepath.Dir(rel) + ":" + f.Name.Name + "."
			collectSuites(f, rel, prefix, shape)
			return nil
		})
		gotest.NoError(t, err)
	}
	return suites
}

func collectSuites(f *ast.File, rel, prefix string, shape func(string) *suiteShape) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !gotestast.IS_TEST_SUITE.MatchString(ts.Name.Name) {
					continue
				}
				s := shape(prefix + ts.Name.Name)
				s.file = rel
				doc := ts.Doc
				if doc == nil {
					doc = d.Doc
				}
				s.reason = hasReason(doc)
			}
		case *ast.FuncDecl:
			name := receiverName(d)
			if name == "" || !gotestast.IS_TEST_SUITE.MatchString(name) {
				continue
			}
			s := shape(prefix + name)
			switch {
			case gotestast.IS_TEST_CASE.MatchString(d.Name.Name):
				s.tests++
			case d.Name.Name == "SuiteConfig" && d.Body != nil:
				s.parallel, s.exclusive = configFlags(d.Body)
			}
		}
	}
}

func receiverName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) != 1 {
		return ""
	}
	typ := d.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func hasReason(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		if reason, ok := strings.CutPrefix(c.Text, "// Sequential:"); ok && strings.TrimSpace(reason) != "" {
			return true
		}
	}
	return false
}

// configFlags reads Parallel and Exclusive from a SuiteConfig body. The
// generator only accepts boolean literals in a keyed literal or a compose
// assignment, so those two forms are all there is to see.
func configFlags(body *ast.BlockStmt) (parallel, exclusive bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		var field string
		var value ast.Expr
		switch x := n.(type) {
		case *ast.KeyValueExpr:
			if key, ok := x.Key.(*ast.Ident); ok {
				field, value = key.Name, x.Value
			}
		case *ast.AssignStmt:
			if sel, ok := x.Lhs[0].(*ast.SelectorExpr); ok && len(x.Rhs) == 1 {
				field, value = sel.Sel.Name, x.Rhs[0]
			}
		}
		if v, ok := value.(*ast.Ident); ok {
			switch field {
			case "Parallel":
				parallel = v.Name == "true"
			case "Exclusive":
				exclusive = v.Name == "true"
			}
		}
		return true
	})
	return parallel, exclusive
}

func (s *SequentialReasonTestSuite) TestEverySequentialSuiteSaysWhy(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	suites := scanSuites(t, root)

	t.When("the repository's suites are scanned", func(w *gotest.T) {
		w.It("finds sequential suites that already state a reason", func(it *gotest.T) {
			runner := suites[filepath.Join("internal", "gotestrunner")+":gotestrunner_test.OverlayTestSuite"]
			gotest.NotZero(it, runner, "the scan missed a known suite")
			gotest.True(it, runner.reason, "the scan missed a known // Sequential: line")
			gotest.Greater(it, runner.tests, 1)
		})

		w.It("leaves no multi-method sequential suite without a // Sequential: line", func(it *gotest.T) {
			var missing []string
			for key, shape := range suites {
				if shape.file == "" || shape.tests < 2 || shape.parallel || shape.exclusive || shape.reason {
					continue
				}
				missing = append(missing, key+" ("+shape.file+")")
			}
			sort.Strings(missing)
			gotest.Empty(it, missing, "make these suites Parallel, or state why not on a // Sequential: line")
		})
	})
}
