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
