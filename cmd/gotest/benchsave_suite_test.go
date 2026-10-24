package main_test

import (
	"os"
	"path/filepath"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchSaveTestSuite: saving to the configured baseline promotes it; the run
// is not compared against the file it is about to replace.
type BenchSaveTestSuite struct{}

func (s *BenchSaveTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

// BeforeAll isolates the temp dir for every method: the runs they make sweep it.
func (s *BenchSaveTestSuite) BeforeAll(t *gotest.T) { isolateTempDir(t) }

// AfterAll has nothing to release: Setenv restores the temp dir.
func (s *BenchSaveTestSuite) AfterAll(_ *gotest.T) {}

func (s *BenchSaveTestSuite) TestABareSaveToTheConfiguredBaseline(t *gotest.T) {
	baseline := filepath.Join(t.TempDir(), "baseline.json")
	inv := main.Invocation{Args: []string{"--save=", "./testdata/census/benchplain/", "-benchtime=1x"}}
	inv.Config.Bench.Baseline = baseline
	inv.Config.Bench.Gate = 10
	code := main.ExportRunBench(inv)
	t.When("bench.baseline and bench.gate are configured and --save= names no path", func(w *gotest.T) {
		w.It("saves the baseline and runs no comparison or gate", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
			_, err := os.Stat(baseline)
			gotest.NoError(it, err)
		})
	})
}
