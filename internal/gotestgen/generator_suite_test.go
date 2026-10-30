package gotestgen_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GeneratorTestSuite tests the code generation pipeline using self-contained
// fixtures in testdata_e2e/.
type GeneratorTestSuite struct{}

func (s *GeneratorTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *GeneratorTestSuite) TestStdlibPackageHasNoSuites(t *gotest.T) {
	t.When("loading a stdlib package", func(w *gotest.T) {
		w.It("loads it, owed a verdict, and generates nothing for it", func(it *gotest.T) {
			loaded, broken, err := gotestgen.LoadPackages([]string{"strings"}, nil)
			gotest.NoError(it, err)
			gotest.Len(it, loaded, 1)
			gotest.Empty(it, broken)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err)
			gotest.Len(it, results, 1)
			gotest.Empty(it, results[0].PTest)
			gotest.Empty(it, results[0].PXTest)
			gotest.Greater(it, results[0].StdlibTestCount, 0)
		})
	})
}

// --- E2E tests (folded from generator_e2e_test.go) ---

func (s *GeneratorTestSuite) TestE2ECLI(t *gotest.T) {
	cases := []struct {
		Desc    string
		dirName string
		hasPX   bool
	}{
		{"no testsuite", "no_testsuite", true},
		{"simple testsuite", "testsuite", true},
		{"suite guard", "suite_guard", false},
		{"fixture lifecycle", "fixture_lifecycle", false},
		{"multi fixture", "multi_fixture", false},
	}

	// One load for every directory. Loading them one at a time re-type-checked
	// the same dependency graph per case, and a single call is what the CLI
	// itself does for a multi-package pattern.
	cwd, err := os.Getwd()
	gotest.NoError(t, err)
	dirs := make([]string, len(cases))
	for i, tC := range cases {
		dirs[i] = filepath.Join(cwd, "testdata_e2e", tC.dirName)
	}

	loaded, broken, err := gotestgen.LoadPackages(dirs, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)
	results, _, err := gotestgen.GenerateFromLoaded(loaded)
	gotest.NoError(t, err)
	gotest.Len(t, results, len(cases), "every directory must generate exactly one result")

	byPath := make(map[string]int, len(results))
	for i, r := range results {
		byPath[r.AbsPath] = i
	}

	t.When("CLI-level generation", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, cases) {
			dirPath := filepath.Join(cwd, "testdata_e2e", tC.dirName)
			i, ok := byPath[dirPath]
			gotest.True(sub, ok, "no generated result for %s", dirPath)
			if !ok {
				continue
			}

			gotest.Equal(sub, dirPath, results[i].AbsPath)
			gotest.Equal(sub, "github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/"+tC.dirName, results[i].PkgPath)

			gotest.MatchSnapshot(sub, string(results[i].PTest), tC.dirName+"-ptest")
			if tC.hasPX {
				gotest.MatchSnapshot(sub, string(results[i].PXTest), tC.dirName+"-pxtest")
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

func (s *GeneratorTestSuite) TestGenerateFromLoaded_TestRunSuiteNames(t *gotest.T) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), "TestGenerator_CaselessSuites")
	results, _, err := gotestgen.GenerateFromLoaded([]*gotestgen.LoadResult{
		{PkgPath: pkg.PkgPath, PkgDir: "/fake/dir", Ptest: pkg},
	})
	gotest.NoError(t, err)
	gotest.Len(t, results, 1)
	r := results[0]

	t.It("schedules suites with test methods or fuzz seeds", func(it *gotest.T) {
		gotest.ElementsMatch(it, []string{"FuzzOnlyTestSuite", "ExcludedWithFuzzTestSuite", "PlainTestSuite"}, r.SuiteNames)
	})

	t.It("marks the suites without a Test function, and not one whose test methods are all excluded", func(it *gotest.T) {
		gotest.Equal(it, []string{"FuzzOnlyTestSuite"}, r.TestlessSuiteNames)
	})

	t.It("reports a suite whose every test method is excluded as skipped", func(it *gotest.T) {
		gotest.ElementsMatch(it, []string{"AllExcludedTestSuite", "BoundExcludedTestSuite"}, r.SkippedSuiteNames)
	})

	t.It("keeps benchmark-only suites for bench runs", func(it *gotest.T) {
		gotest.ElementsMatch(it, []string{"BenchOnlyTestSuite", "BoundBenchTestSuite"}, r.BenchSuiteNames)
	})
}

func (s *GeneratorTestSuite) TestGenerateFromLoaded_CaselessSuites(t *gotest.T) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), "TestCollector_CaselessSuites")
	results, _, err := gotestgen.GenerateFromLoaded([]*gotestgen.LoadResult{
		{PkgPath: pkg.PkgPath, PkgDir: "/fake/dir", Ptest: pkg},
	})

	t.It("generates nothing for a package with a suite that declares no case", func(it *gotest.T) {
		gotest.Empty(it, results)
		gotest.ErrorContains(it, err, "test.go:15:6: suite HelperTestSuite declares no Test, Benchmark or Fuzz method; declare one, or give the helper type a name that does not end in TestSuite")
	})
}

func (s *GeneratorTestSuite) TestE2ENoTestSuites(t *gotest.T) {
	t.When("packages without test suites", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, []struct {
			Desc       string
			arg        string
			wantLoaded int
			wantBroken int
		}{
			{"no test files", "./testdata_e2e/no_testfiles", 0, 0},
			// A pattern that matches nothing is a broken package, not an empty
			// result: a typo'd path must never report a passing run.
			{"non-existent path is broken", "testdata_e2e/nothing-here", 0, 1},
			// A stdlib package with tests loads, to be reported without suites.
			{"stdlib package generates nothing", "strings", 1, 0},
			// Any nested path proves the point, so pick one whose import graph
			// is nearly empty — net/http cost a second of loading to say this.
			{"stdlib nested package generates nothing", "container/heap", 1, 0},
		}) {
			loaded, broken, err := gotestgen.LoadPackages([]string{tC.arg}, nil)
			gotest.NoError(sub, err)
			gotest.Len(sub, loaded, tC.wantLoaded)
			gotest.Len(sub, broken, tC.wantBroken)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(sub, err)
			for _, r := range results {
				gotest.Empty(sub, r.PTest)
				gotest.Empty(sub, r.PXTest)
			}
		}
	})
}
