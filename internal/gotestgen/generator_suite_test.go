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

func (s *GeneratorTestSuite) TestStdlibPackageReturnsEmpty(t *gotest.T) {
	t.When("loading a stdlib package", func(w *gotest.T) {
		w.It("returns empty results", func(it *gotest.T) {
			loaded, err := gotestgen.LoadPackages([]string{"strings"}, nil)
			gotest.NoError(it, err)
			gotest.Empty(it, loaded)
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
		}) {
			cwd, err := os.Getwd()
			gotest.NoError(sub, err)

			dirPath := filepath.Join(cwd, "testdata_e2e", tC.dirName)
			loaded, err := gotestgen.LoadPackages([]string{dirPath}, nil)
			gotest.NoError(sub, err)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(sub, err)
			gotest.True(sub, strings.HasSuffix(filepath.ToSlash(results[0].AbsPath), "go-test/internal/gotestgen/testdata_e2e/"+tC.dirName))
			gotest.Equal(sub, "github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/"+tC.dirName, results[0].PkgPath)

			gotest.MatchSnapshot(sub, string(results[0].PTest), tC.dirName+"-ptest")
			if tC.hasPX {
				gotest.MatchSnapshot(sub, string(results[0].PXTest), tC.dirName+"-pxtest")
			}
		}
	})
}

func (s *GeneratorTestSuite) TestE2ENoTestSuites(t *gotest.T) {
	t.When("packages without test suites", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, []struct {
			Desc string
			arg  string
		}{
			{"no test files", "no_testfiles"},
			{"non-existent path returns empty", "testdata_e2e/nothing-here"},
			{"stdlib package returns empty", "strings"},
			{"stdlib nested package returns empty", "net/http"},
		}) {
			loaded, err := gotestgen.LoadPackages([]string{tC.arg}, nil)
			gotest.NoError(sub, err)
			gotest.Empty(sub, loaded)
		}
	})
}

