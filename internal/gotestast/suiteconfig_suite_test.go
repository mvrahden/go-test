package gotestast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const suiteConfigStubPath = "github.com/mvrahden/go-test/pkg/gotest"

const suiteConfigStub = `package gotest

type SuiteConfig struct {
	Parallel, Exclusive, FailFast bool
	Timeout                       int
}

func DefaultSuiteConfig() SuiteConfig { return SuiteConfig{} }
`

type importerFunc func(path string) (*types.Package, error)

func (fn importerFunc) Import(path string) (*types.Package, error) { return fn(path) }

// parseSuiteConfig type-checks a SuiteConfig method with the given body
// against a stub gotest package and parses that body.
func parseSuiteConfig(t *gotest.T, body string) (*gotestast.SuiteConfigBody, error) {
	fset := token.NewFileSet()
	importer := importerFunc(func(path string) (*types.Package, error) {
		f, err := parser.ParseFile(fset, "gotest.go", suiteConfigStub, 0)
		if err != nil {
			return nil, err
		}
		return (&types.Config{}).Check(suiteConfigStubPath, fset, []*ast.File{f}, nil)
	})
	src := "package testpkg\n\nimport \"" + suiteConfigStubPath + "\"\n\n" +
		"type S struct{}\n\n" +
		"func helper() gotest.SuiteConfig { return gotest.SuiteConfig{} }\n\n" +
		"func (s *S) SuiteConfig() gotest.SuiteConfig {\n" + body + "\n}\n"
	f, err := parser.ParseFile(fset, "fixture.go", src, 0)
	gotest.NoError(t, err)
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	_, err = (&types.Config{Importer: importer}).Check("testpkg", fset, []*ast.File{f}, info)
	gotest.NoError(t, err)

	var fd *ast.FuncDecl
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == "SuiteConfig" {
			fd = d
		}
	}
	gotest.NotZero(t, fd)
	return gotestast.ParseSuiteConfigBody(info, fd)
}

// SuiteConfigBodyTestSuite tests ParseSuiteConfigBody, the SuiteConfig body
// grammar shared by the generator and the linter.
type SuiteConfigBodyTestSuite struct{}

func (s *SuiteConfigBodyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SuiteConfigBodyTestSuite) TestParseSuiteConfigBody(t *gotest.T) {
	t.When("a literal is returned directly", func(w *gotest.T) {
		w.It("describes the literal, its keyed fields and the return holding it", func(it *gotest.T) {
			body, err := parseSuiteConfig(it, "\treturn gotest.SuiteConfig{Parallel: true, Timeout: 3}")
			gotest.NoError(it, err)
			gotest.NotZero(it, body.Literal)
			gotest.Equal(it, ast.Expr(body.Literal), body.Base)
			_, isReturn := body.Holder.(*ast.ReturnStmt)
			gotest.True(it, isReturn)
			gotest.True(it, body.Returned())
			gotest.Empty(it, body.CfgName)
			gotest.Len(it, body.Fields, 2)
			gotest.Empty(it, body.AssignedFields())
		})
	})

	t.When("a preset is returned directly", func(w *gotest.T) {
		w.It("describes the call with no literal", func(it *gotest.T) {
			body, err := parseSuiteConfig(it, "\treturn gotest.DefaultSuiteConfig()")
			gotest.NoError(it, err)
			gotest.Zero(it, body.Literal)
			_, isCall := body.Base.(*ast.CallExpr)
			gotest.True(it, isCall)
			gotest.True(it, body.Returned())
		})
	})

	t.When("the config is composed", func(w *gotest.T) {
		w.It("names the variable and lists the assigned fields in order", func(it *gotest.T) {
			body, err := parseSuiteConfig(it, "\tc := gotest.SuiteConfig{FailFast: true}\n\tc.Parallel = true\n\tc.Timeout = 5\n\treturn c")
			gotest.NoError(it, err)
			_, isAssign := body.Holder.(*ast.AssignStmt)
			gotest.True(it, isAssign)
			gotest.False(it, body.Returned())
			gotest.Equal(it, "c", body.CfgName)
			gotest.Len(it, body.Fields, 1)
			gotest.Equal(it, []string{"Parallel", "Timeout"}, body.AssignedFields())
		})
	})

	t.When("a compose body holds a statement other than a field assignment", func(w *gotest.T) {
		w.It("returns the generator's body error", func(it *gotest.T) {
			_, err := parseSuiteConfig(it, "\tcfg := gotest.SuiteConfig{Parallel: true}\n\t_ = cfg.FailFast\n\treturn cfg")
			gotest.ErrorContains(it, err, "SuiteConfig method body must return a gotest.SuiteConfig literal or preset call")
		})
	})

	t.When("a compose body does not return the config variable", func(w *gotest.T) {
		w.It("returns the generator's body error", func(it *gotest.T) {
			_, err := parseSuiteConfig(it, "\tcfg := gotest.DefaultSuiteConfig()\n\tother := cfg\n\treturn other")
			gotest.ErrorContains(it, err, "SuiteConfig method body must return a gotest.SuiteConfig literal or preset call")
		})
	})

	t.When("the literal is positional", func(w *gotest.T) {
		w.It("requires keyed fields", func(it *gotest.T) {
			_, err := parseSuiteConfig(it, "\treturn gotest.SuiteConfig{true, false, false, 0}")
			gotest.ErrorContains(it, err, "keyed fields")
		})
	})

	t.When("the base is a helper that is not a gotest preset", func(w *gotest.T) {
		w.It("accepts only the gotest presets", func(it *gotest.T) {
			_, err := parseSuiteConfig(it, "\treturn helper()")
			gotest.ErrorContains(it, err, "only the gotest presets")
		})
	})

	t.When("Parallel is not a boolean literal", func(w *gotest.T) {
		w.It("requires a boolean literal", func(it *gotest.T) {
			_, err := parseSuiteConfig(it, "\tcfg := gotest.DefaultSuiteConfig()\n\tcfg.Parallel = !false\n\treturn cfg")
			gotest.ErrorContains(it, err, "Parallel must be assigned a boolean literal")
		})
	})
}
