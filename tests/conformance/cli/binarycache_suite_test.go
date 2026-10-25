package cli_test

import (
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBinaryCacheTestSuite: the test binary cache evicts under load without
// taking a binary from a run that uses it.
type CLIBinaryCacheTestSuite struct{}

func (s *CLIBinaryCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Measured on linux: three rounds of 8 runs ~10s. Not yet measured on
	// Windows CI, whose process starts and file writes are far slower; 3m
	// leaves room.
	return gotest.SuiteConfig{Timeout: 3 * time.Minute}
}

func (s *CLIBinaryCacheTestSuite) TestRunsUnderATinyCap(t *gotest.T) {
	const rounds, runs = 3, 8
	env := []string{protocol.EnvCacheDir + "=" + t.TempDir(), protocol.EnvCacheMaxSize + "=1"}
	var results []testkit.Run
	var errs []error
	for range rounds {
		round := make([]testkit.Run, runs)
		roundErrs := make([]error, runs)
		var wg sync.WaitGroup
		for i := range runs {
			markers := t.TempDir()
			wg.Go(func() {
				round[i], roundErrs[i] = testkit.RunPackageWithEnv(t.Context(), markers, env, "../testdata/passing/")
			})
		}
		wg.Wait()
		results, errs = append(results, round...), append(errs, roundErrs...)
	}

	t.When("8 runs at once, in 3 rounds, share a cache capped at one byte", func(w *gotest.T) {
		w.It("passes every run, each suite running and reporting", func(it *gotest.T) {
			for i, run := range results {
				gotest.NoError(it, errs[i])
				gotest.Equal(it, 0, run.Exit, "run %d stdout: %s\nstderr: %s", i, run.Stdout, run.Stderr)
				gotest.Equal(it, "pass", run.Verdict("TestPassingTestSuite"), "run %d stdout: %s", i, run.Stdout)
			}
		})
	})
}
