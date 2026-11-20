package conventions_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FunctionSizeTestSuite caps the length of every function in the shipped
// packages, so a long function is split before it grows, and the ones
// that are long already cannot grow further.
type FunctionSizeTestSuite struct{}

// ceiling is the length no function may pass, in lines from func to its
// closing brace.
const ceiling = 100

// capped are the functions over the ceiling when it was set, each with a
// ceiling of its own. The list only shrinks.
var capped = map[string]int{
	"cmd/gotest.buildDiscoverSuite":                      110,
	"cmd/gotest.discover":                                120,
	"cmd/gotest.printBenchHelp":                          110,
	"cmd/gotest.runBench":                                300,
	"cmd/gotest.runFuzz":                                 180,
	"cmd/gotest.runFuzzPromote":                          120,
	"cmd/gotest.runFuzzTriage":                           110,
	"cmd/gotest.runLintInProcess":                        130,
	"cmd/gotest.runMigrate":                              110,
	"cmd/gotest.runSpec":                                 190,
	"cmd/gotest.runSummary":                              160,
	"cmd/gotest.watchRunOnce":                            110,
	"internal/drill.drillRun.judge":                      170,
	"internal/drill.runDrill":                            170,
	"internal/gotestast.DetermineTestSuiteHarness":       250,
	"internal/gotestast.Orphans":                         110,
	"internal/gotestast.ValidateContextConsistency":      120,
	"internal/gotestgen.binder.registerSharedFixture":    210,
	"internal/gotestgen.binder.resolveFixture":           130,
	"internal/gotestgen.bind":                            170,
	"internal/gotestgen.collector.CollectSuiteSpecs":     150,
	"internal/gotestgen.fuzzEmitter.emitTarget":          120,
	"internal/gotestgen.fuzzEmitter.fan":                 180,
	"internal/gotestgen.fuzzEmitter.literalHelperSource": 110,
	"internal/gotestgen.generateFromLoaded":              230,
	"internal/gotestgen.loadPackages":                    180,
	"internal/gotestrunner.launchSharedFixtures":         120,
	"internal/gotestrunner.runBench":                     190,
	"internal/gotestspec.BuildTree":                      140,
	"internal/gotestspec.renderNode":                     130,
	"internal/lint.checkFailGuard":                       130,
	"internal/lint.fixer.collect":                        120,
	"internal/lint.load":                                 110,
	"internal/migrate.MigratePackages":                   110,
	"internal/migrate.unit.mapAssertion":                 130,
	"internal/migrate.unit.refuse":                       230,
	"internal/migrate.unit.transformFile":                130,
	"pkg/gotest.matchSnapshot":                           120,
	"pkg/gotest/internal/assert.renderer.flat":           130,
	"pkg/gotest/internal/assert.renderer.tree":           120,
	"pkg/gotestruntime.setupDAG":                         110,
}

// functionLengths measures every function with a body in the non-test Go
// files under roots, keyed <dir>.<Name> or <dir>.<Recv>.<Name>, dir
// relative to root. Build-tagged twins keep the longer.
func functionLengths(t *gotest.T, root string, roots ...string) map[string]int {
	lengths := map[string]int{}
	fset := token.NewFileSet()
	for _, r := range roots {
		err := filepath.WalkDir(filepath.Join(root, r), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				key := filepath.ToSlash(rel) + "."
				if fn.Recv != nil && len(fn.Recv.List) > 0 {
					key += receiverType(fn.Recv.List[0].Type) + "."
				}
				key += fn.Name.Name
				n := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
				lengths[key] = max(lengths[key], n)
			}
			return nil
		})
		gotest.NoError(t, err)
	}
	return lengths
}

// receiverType is the receiver's type name, pointer and type parameters
// stripped.
func receiverType(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return fmt.Sprintf("%T", e)
		}
	}
}

func (s *FunctionSizeTestSuite) TestNoFunctionPassesItsCeiling(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	lengths := functionLengths(t, root, "cmd", "internal", "pkg")
	var over, stale []string
	for name, n := range lengths {
		limit, named := capped[name]
		if !named {
			limit = ceiling
		}
		if n > limit {
			over = append(over, fmt.Sprintf("%s is %d lines, its ceiling is %d", name, n, limit))
		}
	}
	for name, limit := range capped {
		switch n, ok := lengths[name]; {
		case !ok:
			stale = append(stale, name+" no longer exists: delete its entry")
		case n <= ceiling:
			stale = append(stale, fmt.Sprintf("%s is %d lines, under the ceiling of %d every function has: delete its entry (was %d)", name, n, ceiling, limit))
		}
	}
	slices.Sort(over)
	slices.Sort(stale)
	gotest.Empty(t, over, "split the function at a boundary of its own; a ceiling in tests/conventions/funcsize_suite_test.go is never raised")
	gotest.Empty(t, stale)
}
