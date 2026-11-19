package lifecycle_test

import (
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AtRestTestSuite: a suite process under the runner allocates nothing of its
// own accord while its test waits.
type AtRestTestSuite struct{}

func (s *AtRestTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 90 * time.Second}
}

func (s *AtRestTestSuite) TestAMethodThatOnlyWaits(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/atrest/")
	gotest.NoError(t, err)
	gotest.Equal(t, "pass", run.Verdict("TestAtRestTestSuite/TestWaits"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)

	t.When("a method of a suite under the runner only sleeps while it counts the process's heap allocations", func(w *gotest.T) {
		w.It("counts at most 10 more in a window one second longer than in a short one", func(it *gotest.T) {
			short, long := markedNumber(run.Markers, "short="), markedNumber(run.Markers, "long=")
			gotest.GreaterOrEqual(it, short, 0, "markers: %v", run.Markers)
			gotest.LessOrEqual(it, long, short+10, "markers: %v", run.Markers)
		})
		w.It("gets 0 from testing.AllocsPerRun of a function that only sleeps", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "allocs per run=0")
		})
	})
}

// markedNumber reads the number a marker with prefix carries, -1 without one.
func markedNumber(markers []string, prefix string) int {
	for _, m := range markers {
		if n, err := strconv.Atoi(strings.TrimPrefix(m, prefix)); err == nil && strings.HasPrefix(m, prefix) {
			return n
		}
	}
	return -1
}
