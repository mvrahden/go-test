package gotestgen_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GeneratorTestSuite tests the code generation pipeline using self-contained
// fixtures in testdata_e2e/.
type GeneratorTestSuite struct{}

// SuiteConfig: the end-to-end fixtures are generated and built, which
// the 30-second default does not cover on a slow machine.
func (s *GeneratorTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

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

// The package and its external test package build into one binary, which
// allows one TestMain; both variants still register their own DAG's teardown.
func (s *GeneratorTestSuite) TestTestMainOwner(t *gotest.T) {
	cwd, err := os.Getwd()
	gotest.NoError(t, err)
	// dir is relative to the package. Packages under testdata_e2e build and run
	// with the module's tests; one that must not run lives under testdata.
	gen := func(it *gotest.T, dir string) *gotestgen.GenerateResult {
		loaded, broken, err := gotestgen.LoadPackages([]string{filepath.Join(cwd, dir)}, nil)
		gotest.NoError(it, err)
		gotest.Empty(it, broken)
		results, _, err := gotestgen.GenerateFromLoaded(loaded)
		gotest.NoError(it, err)
		gotest.Len(it, results, 1)
		return results[0]
	}
	const testMain = "func TestMain(m *testing.M)"
	const register = "gotestruntime.RegisterTeardown(ƒ_fixtureDAG.Teardown)"

	t.When("both variants bind fixtures", func(w *gotest.T) {
		w.It("emits the TestMain in the package's own variant only", func(it *gotest.T) {
			r := gen(it, "testdata_e2e/testmain_both")
			gotest.Equal(it, 1, strings.Count(string(r.PTest), testMain))
			gotest.NotContains(it, string(r.PXTest), testMain)
			gotest.Contains(it, string(r.PTest), register)
			gotest.Contains(it, string(r.PXTest), register)
		})
	})

	t.When("the developer wrote a TestMain that calls the runtime", func(w *gotest.T) {
		w.It("keeps theirs and emits none", func(it *gotest.T) {
			r := gen(it, "testdata_e2e/testmain_user")
			gotest.NotContains(it, string(r.PTest), testMain)
			gotest.NotContains(it, string(r.PXTest), testMain)
			gotest.Contains(it, string(r.PTest), register)
		})
	})

	// Whether a TestMain routes the tests through the runtime is decided at run
	// time, where it is known exactly; generation neither refuses nor guesses.
	t.When("the developer wrote a TestMain that calls m.Run", func(w *gotest.T) {
		w.It("generates, emits no TestMain, and leaves the check to fixture setup", func(it *gotest.T) {
			r := gen(it, "testdata/testmain_run")
			gotest.NotContains(it, string(r.PTest), testMain)
			gotest.Contains(it, string(r.PTest), "gotestruntime.RequireMain()")
		})
	})

	t.When("only the external test package binds fixtures", func(w *gotest.T) {
		w.It("emits the TestMain there", func(it *gotest.T) {
			r := gen(it, "testdata_e2e/testmain_xonly")
			gotest.NotContains(it, string(r.PTest), testMain)
			gotest.Equal(it, 1, strings.Count(string(r.PXTest), testMain))
		})
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
			// Any nested path proves the point, so pick one whose import graph
			// is nearly empty — net/http cost a second of loading to say this.
			{"stdlib nested package returns empty", "container/heap", 0},
		}) {
			loaded, broken, err := gotestgen.LoadPackages([]string{tC.arg}, nil)
			gotest.NoError(sub, err)
			gotest.Empty(sub, loaded)
			gotest.Len(sub, broken, tC.wantBroken)
		}
	})
}
