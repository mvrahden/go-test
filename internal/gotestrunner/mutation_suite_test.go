package gotestrunner_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// MutationRunTestSuite: what a run takes from audit --mutate's variables:
// the mutants' overlay, given to its suites in GOFLAGS, and each suite's
// coverage kept apart.
//
// Sequential: Setenv.
type MutationRunTestSuite struct{}

func (s *MutationRunTestSuite) TestTheMutantsOverlay(t *gotest.T) {
	dir := t.TempDir()
	overlay := filepath.Join(dir, "overlay.json")
	gotest.NoError(t, os.WriteFile(overlay, []byte(`{"Replace":{"/m/a.go":"/x/0_a.go"}}`), 0o600))
	t.When("no overlay is named", func(w *gotest.T) {
		w.It("adds nothing and leaves GOFLAGS", func(it *gotest.T) {
			it.Setenv("GOTEST_MUTANT_OVERLAY", "")
			replace, err := gotestrunner.ExportMutantOverlay()
			gotest.NoError(it, err)
			gotest.Empty(it, replace)
			gotest.Equal(it, []string{"GOFLAGS=-v"}, gotestrunner.ExportWithMutantFlags([]string{"GOFLAGS=-v"}))
		})
	})
	t.When("an overlay is named", func(w *gotest.T) {
		w.It("reads its replacements and adds it to the suites' GOFLAGS", func(it *gotest.T) {
			it.Setenv("GOTEST_MUTANT_OVERLAY", overlay)
			replace, err := gotestrunner.ExportMutantOverlay()
			gotest.NoError(it, err)
			gotest.Equal(it, map[string]string{"/m/a.go": "/x/0_a.go"}, replace)
			gotest.Equal(it, []string{"PATH=/bin", "GOFLAGS=-tags=x -overlay=" + overlay}, gotestrunner.ExportWithMutantFlags([]string{"GOFLAGS=-tags=x", "PATH=/bin"}))
			gotest.Equal(it, []string{"GOFLAGS=-overlay=" + overlay}, gotestrunner.ExportWithMutantFlags(nil))
		})
	})
	t.When("the overlay is missing or no JSON", func(w *gotest.T) {
		w.It("fails naming the variable", func(it *gotest.T) {
			it.Setenv("GOTEST_MUTANT_OVERLAY", filepath.Join(dir, "absent.json"))
			_, err := gotestrunner.ExportMutantOverlay()
			gotest.ErrorContains(it, err, "GOTEST_MUTANT_OVERLAY: ")
			bad := filepath.Join(dir, "bad.json")
			gotest.NoError(it, os.WriteFile(bad, []byte("{"), 0o600))
			it.Setenv("GOTEST_MUTANT_OVERLAY", bad)
			_, err = gotestrunner.ExportMutantOverlay()
			gotest.ErrorContains(it, err, "GOTEST_MUTANT_OVERLAY: "+bad+": ")
		})
	})
}

func (s *MutationRunTestSuite) TestEachSuitesCoverage(t *gotest.T) {
	t.When("the run keeps each suite's coverage apart", func(w *gotest.T) {
		w.It("gives each suite a GOCOVERDIR beside its profile and lists them", func(it *gotest.T) {
			dir := it.TempDir()
			it.Setenv("GOTEST_SUITE_COVERAGE", dir)
			envs := gotestrunner.ExportSuiteCoverage([]string{"GOCOVERDIR=/run", "PATH=/bin"}, [][3]string{
				{"m/a", "TestS", filepath.Join(dir, "0.out")},
			})
			gotest.Equal(it, [][]string{{"PATH=/bin", "GOCOVERDIR=" + filepath.Join(dir, "0.covdata")}}, envs)
			info, err := os.Stat(filepath.Join(dir, "0.covdata"))
			gotest.NoError(it, err)
			gotest.True(it, info.IsDir())
			list, err := gotestrunner.ReadSuiteCoverage(dir)
			gotest.NoError(it, err)
			gotest.Equal(it, []gotestrunner.SuiteCoverage{
				{Package: "m/a", Suite: "TestS", Profile: filepath.Join(dir, "0.out"), CoverDir: filepath.Join(dir, "0.covdata")},
			}, list)
		})
	})
	t.When("the run does not keep them apart", func(w *gotest.T) {
		w.It("leaves the environment and writes no list", func(it *gotest.T) {
			it.Setenv("GOTEST_SUITE_COVERAGE", "")
			envs := gotestrunner.ExportSuiteCoverage([]string{"GOCOVERDIR=/run"}, [][3]string{{"m/a", "TestS", filepath.Join(it.TempDir(), "0.out")}})
			gotest.Equal(it, [][]string{{"GOCOVERDIR=/run"}}, envs)
			_, err := gotestrunner.ReadSuiteCoverage(it.TempDir())
			gotest.Error(it, err)
		})
	})
}
