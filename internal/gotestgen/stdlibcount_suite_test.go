package gotestgen_test

import (
	"go/ast"
	"go/parser"
	"go/token"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StdlibCountTestSuite covers which stdlib functions count as tests gotest
// does not run.
type StdlibCountTestSuite struct{}

func (s *StdlibCountTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const stdlibCountSrc = `package p

import "testing"

func TestParse(t *testing.T)      {}
func Testify(t *testing.T)        {}
func BenchmarkParse(b *testing.B) {}
func Benchmarked(b *testing.B)    {}
func FuzzParse(f *testing.F)      {}
func Fuzzy(f *testing.F)          {}
`

func (s *StdlibCountTestSuite) TestNameRule(t *gotest.T) {
	t.When("a function continues its prefix with a lowercase letter", func(w *gotest.T) {
		w.It("is not counted, as go test does not run it", func(it *gotest.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p_test.go", stdlibCountSrc, 0)
			gotest.NoError(it, err)
			pkg := &packages.Package{Fset: fset, Syntax: []*ast.File{f}}
			gotest.Equal(it, 3, gotestgen.ExportCountStdlibTests(pkg))
		})
	})
}
