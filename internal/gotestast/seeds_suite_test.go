package gotestast_test

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// loadHarvestTestPkgs loads the real, module-resolved testdata/seeds/harvest
// package (both its internal/ptest and external/pxtest test-binary variants)
// via golang.org/x/tools/go/packages with Tests: true, so that gotest.Each
// and gotest.Fuzz/Fuzz2/Fuzz3 call sites resolve to their real *types.Func
// identities — the ad-hoc types.Config{Importer: importer.Default()} setup
// used elsewhere in this file can't resolve non-stdlib imports. Tests: true
// (and picking the ".test]" variants) is essential here: it's what makes
// pkg.Syntax bundle production (prod.go) and test (harvest_test.go) files
// together, mirroring exactly how gotestgen.LoadPackages loads Ptest/Pxtest
// for real generation — the shape HarvestSeeds must filter correctly.
func loadHarvestTestPkgs(t *testing.T) (ptest, pxtest *packages.Package) {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   ".",
	}
	pkgs, err := packages.Load(cfg, "./testdata/seeds/harvest")
	gotest.NoError(t, err)
	for _, p := range pkgs {
		gotest.Empty(t, p.Errors, "package load errors for %s: %v", p.ID, p.Errors)
	}
	for _, p := range pkgs {
		if !strings.HasSuffix(p.ID, ".test]") {
			continue // skip the non-test-binary variant and the synthesized "*.test" main
		}
		if strings.HasSuffix(p.Name, "_test") {
			pxtest = p
		} else {
			ptest = p
		}
	}
	gotest.NotZero(t, ptest, "expected to find the ptest package variant")
	gotest.NotZero(t, pxtest, "expected to find the pxtest package variant")
	return ptest, pxtest
}

// collectHarvestSuites re-implements the minimal slice of collector.go's
// suite/harness discovery needed here — importing internal/gotestgen from
// gotestast's own tests would invert the package layering.
func collectHarvestSuites(t *testing.T, pkg *packages.Package) gotestast.TestSuiteSpecSet {
	t.Helper()
	var suites gotestast.TestSuiteSpecSet
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			spec, _, err := gotestast.DetermineTestSuite(gd, pkg)
			gotest.NoError(t, err)
			if spec != nil {
				suites = append(suites, spec)
			}
		}
	}
	for _, spec := range suites {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				_, err := gotestast.DetermineTestSuiteHarness(fd, pkg, spec)
				gotest.NoError(t, err)
			}
		}
	}
	return suites
}

// SeedsTestSuite tests HarvestSeeds against the testdata/seeds/harvest
// fixture package: a table test + a plain literal call site both feed the
// same Parse function a fuzz callback invokes, and a generic Echo callee
// exercises the type-mismatch skip path. prod.go additionally carries a
// literal Parse(...) call from PRODUCTION code, and an external_pxtest_test.go
// file carries a table literal in a separate *packages.Package — both must
// never be harvested.
type SeedsTestSuite struct{}

func (s *SeedsTestSuite) TestHarvestSeeds(t *gotest.T) {
	ptestPkg, _ := loadHarvestTestPkgs(t.T())
	suites := collectHarvestSuites(t.T(), ptestPkg)
	gotest.Len(t, suites, 3)

	seeds, err := gotestast.HarvestSeeds(ptestPkg, suites)
	gotest.NoError(t, err)

	t.When("a fuzz callback's callee is exercised by an Each table and a literal It-block call", func(w *gotest.T) {
		got := seeds["FuzzParseTestSuite_FuzzParse"]

		w.It("harvests exactly the 3 qualifying literals", func(it *gotest.T) {
			gotest.Len(it, got, 3)
		})

		w.It("harvests the two literal table rows and skips the non-literal row", func(it *gotest.T) {
			var args []string
			for _, sl := range got {
				gotest.Len(it, sl.Args, 1)
				args = append(args, sl.Args[0])
			}
			gotest.Contains(it, args, `"5"`)
			gotest.Contains(it, args, `"42"`)
			gotest.NotContains(it, args, "computedInput")
		})

		w.It("harvests the direct literal call from the It block", func(it *gotest.T) {
			var args []string
			for _, sl := range got {
				args = append(args, sl.Args[0])
			}
			gotest.Contains(it, args, `"literal input"`)
		})

		w.It("never harvests the literal Parse(...) call from production code (prod.go)", func(it *gotest.T) {
			var args []string
			for _, sl := range got {
				args = append(args, sl.Args[0])
			}
			gotest.NotContains(it, args, `"from production code — must not be harvested"`)
		})
	})

	t.When("a fuzz callback takes a struct type", func(w *gotest.T) {
		w.It("harvests nothing, even from matching composite-literal rows", func(it *gotest.T) {
			// Load-bearing invariant, not a coverage gap: the generated
			// wrapper adds harvested seeds through raw *testing.F.Add
			// BEFORE the codec-carrying *gotest.F exists (gotest.fuzz.tpl),
			// where a struct seed panics with "unsupported type to Add".
			// Widening the harvester to composite literals (Phase B)
			// therefore requires moving that routing behind *gotest.F.Add
			// first — this assertion is the tripwire.
			got, ok := seeds["FuzzMsgTestSuite_FuzzHandleMsg"]
			gotest.False(it, ok)
			gotest.Empty(it, got)
		})
	})

	t.When("two fuzz callbacks share one generic callee with different param types", func(w *gotest.T) {
		w.It("harvests the literal for the type that matches", func(it *gotest.T) {
			got := seeds["FuzzEchoTestSuite_FuzzEchoString"]
			gotest.Len(it, got, 1)
			gotest.Equal(it, []string{`"only a string"`}, got[0].Args)
		})

		w.It("skips the same literal for the type that does not match", func(it *gotest.T) {
			got, ok := seeds["FuzzEchoTestSuite_FuzzEchoInt"]
			gotest.False(it, ok)
			gotest.Empty(it, got)
		})
	})
}

// TestHarvestSeeds_OrderingIsStable asserts two consecutive HarvestSeeds
// calls over the same package return identical, identically-ordered Args
// slices — the generated f.Add(...) lines must not flap between runs (which
// would make generated output diff noisily / break content-addressed
// overlay caching).
func (s *SeedsTestSuite) TestHarvestSeedsOrderingIsStable(t *gotest.T) {
	ptestPkg, _ := loadHarvestTestPkgs(t.T())
	suites := collectHarvestSuites(t.T(), ptestPkg)

	first, err := gotestast.HarvestSeeds(ptestPkg, suites)
	gotest.NoError(t, err)
	second, err := gotestast.HarvestSeeds(ptestPkg, suites)
	gotest.NoError(t, err)

	gotest.Len(t, first, len(second))
	for funcName, firstSeeds := range first {
		secondSeeds, ok := second[funcName]
		gotest.True(t, ok, "missing %s in second run", funcName)
		gotest.Len(t, secondSeeds, len(firstSeeds))
		for i := range firstSeeds {
			gotest.Equal(t, firstSeeds[i].Args, secondSeeds[i].Args, "%s seed %d: first=%v second=%v", funcName, i, firstSeeds[i].Args, secondSeeds[i].Args)
		}
	}
}

// TestHarvestSeeds_PtestPxtestBoundary documents current, deliberate
// behavior: harvesting never crosses *packages.Package boundaries. FuzzParse
// lives in the ptest (internal) "harvest" package; a second Each table
// calling the same harvest.Parse with a distinct literal lives in the pxtest
// (external "harvest_test") package. Calling HarvestSeeds with the ptest
// package must not see the pxtest-only literal.
func (s *SeedsTestSuite) TestHarvestSeedsPtestPxtestBoundary(t *gotest.T) {
	ptestPkg, _ := loadHarvestTestPkgs(t.T())
	suites := collectHarvestSuites(t.T(), ptestPkg)

	seeds, err := gotestast.HarvestSeeds(ptestPkg, suites)
	gotest.NoError(t, err)

	got := seeds["FuzzParseTestSuite_FuzzParse"]
	var args []string
	for _, sl := range got {
		args = append(args, sl.Args[0])
	}
	gotest.NotContains(t, args, `"external-only"`)
}
