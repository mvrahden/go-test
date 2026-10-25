package cli_test

import (
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIOverlayTestSuite: the files a run generates are its own, so runs that
// share a cache cannot break each other's.
type CLIOverlayTestSuite struct{}

func (s *CLIOverlayTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Measured on linux: the 16-run method ~6s. Not yet measured on Windows
	// CI, whose process starts and file writes are far slower; 2m leaves room.
	return gotest.SuiteConfig{Timeout: 2 * time.Minute}
}

func (s *CLIOverlayTestSuite) TestConcurrentRunsSharingACache(t *gotest.T) {
	const runs = 16
	cache := t.TempDir()
	results := make([]testkit.Run, runs)
	errs := make([]error, runs)
	var wg sync.WaitGroup
	for i := range runs {
		markers := t.TempDir()
		wg.Go(func() {
			results[i], errs[i] = testkit.RunPackageWithEnv(t.Context(), markers, []string{protocol.EnvCacheDir + "=" + cache}, "../testdata/passing/")
		})
	}
	wg.Wait()

	t.When("16 runs of one package start at once and share one cache", func(w *gotest.T) {
		w.It("passes every run", func(it *gotest.T) {
			for i := range runs {
				gotest.NoError(it, errs[i])
				gotest.Equal(it, 0, results[i].Exit, "run %d stdout: %s\nstderr: %s", i, results[i].Stdout, results[i].Stderr)
			}
		})
	})
}
