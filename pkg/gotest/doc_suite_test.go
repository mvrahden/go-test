package gotest_test

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DocTestSuite: the packages a user meets on pkg.go.dev say what they are,
// and the names a test is written against say what they do.
type DocTestSuite struct{}

func (s *DocTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// docOf parses the non-test files of dir and returns their documentation.
func docOf(t *gotest.T, dir string) *doc.Package {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	gotest.NoError(t, err)
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		gotest.NoError(t, err)
		files = append(files, f)
	}
	pkg, err := doc.NewFromFiles(fset, files, "example.invalid/"+filepath.Base(dir))
	gotest.NoError(t, err)
	return pkg
}

func (s *DocTestSuite) TestPackageComments(t *gotest.T) {
	for sub, row := range gotest.Each(t, []struct{ Desc, dir, opens string }{
		{"gotest", ".", "Package gotest is what a gotest suite is written against"},
		{"gotestruntime", "../gotestruntime", "Package gotestruntime is what the harness gotest generates calls"},
		{"gotestfuzz", "../gotestfuzz", "Package gotestfuzz holds the byte codecs"},
	}) {
		gotest.Regexp(sub, "^"+regexp.QuoteMeta(row.opens), docOf(sub, row.dir).Doc)
	}
}

func (s *DocTestSuite) TestTheNamesATestIsWrittenAgainst(t *gotest.T) {
	pkg := docOf(t, ".")
	documented := map[string]string{}
	for _, f := range pkg.Funcs {
		documented[f.Name] = f.Doc
	}
	for _, typ := range pkg.Types {
		documented[typ.Name] = typ.Doc
		for _, f := range typ.Funcs {
			documented[f.Name] = f.Doc
		}
		for _, m := range typ.Methods {
			documented[typ.Name+"."+m.Name] = m.Doc
		}
	}
	for _, name := range []string{
		"T", "NewT", "T.Context", "T.Errorf", "T.FailNow", "T.Skipf", "T.It", "T.When", "MatchSnapshot",
		"B", "B.B", "B.Name", "B.Loop", "B.ReportAllocs", "B.SetBytes", "B.ReportMetric", "B.Context", "B.Errorf", "B.FailNow", "B.Skipf",
	} {
		gotest.NotEmpty(t, strings.TrimSpace(documented[name]), "%s has no doc comment", name)
	}
}
