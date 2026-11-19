package perf_test

import (
	"slices"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CountsTestSuite holds a warm run to the go commands it starts and the
// test-binary processes it starts per suite.
type CountsTestSuite struct {
	st staging
}

// Exclusive, as every perf suite: the CPU suite measures with nothing else
// of the run beside it.
func (s *CountsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 2 * time.Minute, SetupTimeout: 3 * time.Minute}
}

func (s *CountsTestSuite) BeforeAll(t *gotest.T) { s.st.setUp(t) }

func (s *CountsTestSuite) AfterAll(t *gotest.T) { s.st.tearDown(t) }

func (s *CountsTestSuite) TestGoInvocations(t *gotest.T) {
	m12 := s.st.warm(t, s.st.m12, "./g/...")
	m6 := s.st.warm(t, s.st.m6, "./g/...")
	testC := func(calls [][]string) int {
		return countGo(calls, func(a []string) bool { return len(a) > 1 && a[0] == "test" && slices.Contains(a, "-c") })
	}
	t.It("starts one go test -c per group of binary names", func(it *gotest.T) {
		want := 12
		if goTestCPerGroup {
			want = 1
		}
		gotest.Equal(it, want, testC(m12.Go))
	})
	t.It("starts no more go commands for twelve packages than for six, beyond the budget", func(it *gotest.T) {
		gotest.Equal(it, 6*goCallsPerExtraPackage, len(m12.Go)-len(m6.Go))
	})
	t.It("lists no test variant for export data", func(it *gotest.T) {
		n := countGo(m12.Go, func(a []string) bool {
			return a[0] == "list" && slices.Contains(a, "-test=true") && slices.Contains(a, "-export=true")
		})
		gotest.Equal(it, testVariantExportLoads, n)
	})
}

func (s *CountsTestSuite) TestProcessesPerSuite(t *gotest.T) {
	m, err := s.st.h.measure(t.Context(), s.st.m12, s.st.cli, s.st.env, "./g/...", "-exec", s.st.execCount)
	gotest.NoError(t, err)
	t.It("starts one process per suite", func(it *gotest.T) {
		gotest.Equal(it, 0, m.Exit, "stderr: %s", m.Stderr)
		gotest.Equal(it, 12*2*execsPerSuite, m.Execs)
	})
	filtered, err := s.st.h.measure(t.Context(), s.st.m12, s.st.cli, s.st.env, "./g/...", "-exec", s.st.execCount, "-run", "^TestS00TestSuite$/^TestM01$")
	gotest.NoError(t, err)
	t.It("starts one process per suite the filter selects", func(it *gotest.T) {
		gotest.Equal(it, 0, filtered.Exit, "stderr: %s", filtered.Stderr)
		gotest.Equal(it, 12*execsPerSuite, filtered.Execs)
	})
}
