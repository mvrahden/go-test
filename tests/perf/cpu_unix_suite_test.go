//go:build unix

package perf_test

import (
	"slices"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CPUTestSuite holds a warm, no-change run to a band over go test -count=1
// on the stdlib twin. Unix only: Windows has no child CPU accounting in
// os.ProcessState, and the count suites are its gate.
type CPUTestSuite struct {
	st staging
}

// Exclusive: it measures CPU, with nothing else of the run beside it.
func (s *CPUTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 2 * time.Minute, SetupTimeout: 3 * time.Minute}
}

func (s *CPUTestSuite) BeforeAll(t *gotest.T) { s.st.setUp(t) }

func (s *CPUTestSuite) AfterAll(t *gotest.T) { s.st.tearDown(t) }

func (s *CPUTestSuite) TestCPURelativeToGoTest(t *gotest.T) {
	s.st.warm(t, s.st.m12, "./g/...")
	// Warm the twin's build cache.
	_, err := s.st.h.measure(t.Context(), s.st.m12, "go", nil, "test", "-count=1", "./s/...")
	gotest.NoError(t, err)
	var ratios []float64
	for range 5 {
		g, err := s.st.h.measure(t.Context(), s.st.m12, s.st.cli, s.st.env, "./g/...")
		gotest.NoError(t, err)
		w, err := s.st.h.measure(t.Context(), s.st.m12, "go", nil, "test", "-count=1", "./s/...")
		gotest.NoError(t, err)
		gotest.Equal(t, 0, g.Exit, "stderr: %s", g.Stderr)
		gotest.Equal(t, 0, w.Exit, "stderr: %s", w.Stderr)
		gotest.Greater(t, w.CPU, time.Duration(0))
		ratios = append(ratios, float64(g.CPU)/float64(w.CPU))
	}
	slices.Sort(ratios)
	t.It("stays within the band", func(it *gotest.T) {
		gotest.LessOrEqual(it, ratios[2], cpuBand, "ratios: %v", ratios)
	})
}
