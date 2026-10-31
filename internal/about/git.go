package about

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
)

const (
	PSuite  = "gotest_psuite_test.go"
	PXSuite = "gotest_pxsuite_test.go"
)

// GeneratedFileNames are every name a generated file may have on disk.
var GeneratedFileNames = []string{PSuite, PXSuite}

// PSuiteRegex matches a generated file's name, alone or ending a path.
var PSuiteRegex = regexp.MustCompile(`(?:^|[/\\])gotest_p(x)?suite_test\.go$`)

const (
	Application = "gotest"
	Repo        = "github.com/mvrahden/go-test"
)

var Version = "dev"

// ParseStubbingGenerated is a go/packages ParseFile that reads a generated
// file as its package clause alone: one left on disk may name suites since
// renamed, and the harness is generated anew anyway.
func ParseStubbingGenerated(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
	const mode = parser.AllErrors | parser.ParseComments
	if PSuiteRegex.MatchString(filepath.Base(filename)) {
		if stub := PackageStub(src); stub != nil {
			return parser.ParseFile(fset, filename, stub, mode)
		}
	}
	return parser.ParseFile(fset, filename, src, mode)
}

// IsGeneratedSource reports whether src carries Go's generated-code
// comment before its package clause.
func IsGeneratedSource(src []byte) bool {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.PackageClauseOnly|parser.ParseComments)
	return err == nil && ast.IsGenerated(f)
}

// PackageStub is src cut down to its package clause and what precedes it,
// build constraints included, or nil when src has none.
func PackageStub(src []byte) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.PackageClauseOnly)
	if err != nil || f.Name == nil {
		return nil
	}
	head := src[:fset.Position(f.Package).Offset]
	return append(append([]byte{}, head...), "package "+f.Name.Name+"\n"...)
}
