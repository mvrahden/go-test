package gotestgen_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GeneratorTestSuite tests the code generation pipeline using self-contained
// fixtures in testdata_e2e/.
type GeneratorTestSuite struct{}

func (s *GeneratorTestSuite) TestStdlibPackageReturnsEmpty(t *gotest.T) {
	t.When("loading a stdlib package", func(w *gotest.T) {
		w.It("returns empty results", func(it *gotest.T) {
			loaded, broken, err := gotestgen.LoadPackages([]string{"strings"}, nil)
			gotest.NoError(it, err)
			gotest.Empty(it, loaded)
			gotest.Empty(it, broken)
		})
	})
}

// --- E2E tests (folded from generator_e2e_test.go) ---

func (s *GeneratorTestSuite) TestE2ECLI(t *gotest.T) {
	t.When("CLI-level generation", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, []struct {
			Desc    string
			dirName string
			hasPX   bool
		}{
			{"no testsuite", "no_testsuite", true},
			{"simple testsuite", "testsuite", true},
			{"suite guard", "suite_guard", false},
			{"fixture lifecycle", "fixture_lifecycle", false},
			{"multi fixture", "multi_fixture", false},
		}) {
			cwd, err := os.Getwd()
			gotest.NoError(sub, err)

			dirPath := filepath.Join(cwd, "testdata_e2e", tC.dirName)
			loaded, broken, err := gotestgen.LoadPackages([]string{dirPath}, nil)
			gotest.NoError(sub, err)
			gotest.Empty(sub, broken)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(sub, err)
			gotest.Equal(sub, dirPath, results[0].AbsPath)
			gotest.Equal(sub, "github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/"+tC.dirName, results[0].PkgPath)

			gotest.MatchSnapshot(sub, string(results[0].PTest), tC.dirName+"-ptest")
			if tC.hasPX {
				gotest.MatchSnapshot(sub, string(results[0].PXTest), tC.dirName+"-pxtest")
			}
		}
	})
}

func (s *GeneratorTestSuite) TestGenerateFromLoaded_BenchSuiteNames(t *gotest.T) {
	t.When("a suite has effective benchmark methods", func(w *gotest.T) {
		w.It("includes the suite identifier in BenchSuiteNames", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BenchmarkMethod")
			loaded := []*gotestgen.LoadResult{
				{PkgPath: pkg.PkgPath, PkgDir: "/fake/dir", Ptest: pkg},
			}
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err)
			gotest.Len(it, results, 1)
			gotest.Contains(it, results[0].SuiteNames, "BenchTestSuite")
			gotest.Contains(it, results[0].BenchSuiteNames, "BenchTestSuite")
		})
	})

	t.When("a suite has no benchmark methods", func(w *gotest.T) {
		w.It("excludes the suite identifier from BenchSuiteNames", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_Detected")
			loaded := []*gotestgen.LoadResult{
				{PkgPath: pkg.PkgPath, PkgDir: "/fake/dir", Ptest: pkg},
			}
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err)
			gotest.Len(it, results, 1)
			gotest.Empty(it, results[0].BenchSuiteNames)
		})
	})
}

func (s *GeneratorTestSuite) TestE2ENoTestSuites(t *gotest.T) {
	t.When("packages without test suites", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, []struct {
			Desc       string
			arg        string
			wantBroken int
		}{
			{"no test files", "./testdata_e2e/no_testfiles", 0},
			// A pattern that matches nothing is a broken package, not an empty
			// result: a typo'd path must never report a passing run.
			{"non-existent path is broken", "testdata_e2e/nothing-here", 1},
			{"stdlib package returns empty", "strings", 0},
			{"stdlib nested package returns empty", "net/http", 0},
		}) {
			loaded, broken, err := gotestgen.LoadPackages([]string{tC.arg}, nil)
			gotest.NoError(sub, err)
			gotest.Empty(sub, loaded)
			gotest.Len(sub, broken, tC.wantBroken)
		}
	})
}
