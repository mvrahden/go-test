package bench_test

import (
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchDeadlineTestSuite: an expired --timeout in a bench run names the
// benchmark it cut short, not only the suite around it.
type BenchDeadlineTestSuite struct{}

func (s *BenchDeadlineTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~66s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Timeout: 3 * time.Minute}
}

func (s *BenchDeadlineTestSuite) TestAHungBenchmark(t *gotest.T) {
	// One cache for both runs: the first, under which nothing hangs, builds
	// the binary, so the timeout covers the benchmarks alone.
	cache := protocol.EnvCacheDir + "=" + filepath.Join(t.TempDir(), "cache")
	_, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{cache, "LCB_WARM=1"}, "bench", "../testdata/benchhang/", "-benchtime=1x")
	gotest.NoError(t, err)
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{cache}, "bench", "--timeout=10s", "../testdata/benchhang/", "-benchtime=1x")
	gotest.NoError(t, err)

	t.When("a benchmark hangs until --timeout in a text bench run", func(w *gotest.T) {
		w.It("names the benchmark as running when the timeout expired", func(it *gotest.T) {
			gotest.Regexp(it, `while running: \S+/benchhang BenchmarkHangTestSuite/BenchmarkHang\n`, run.Stderr, "stdout: %s", run.Stdout)
		})
	})
}
