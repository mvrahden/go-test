package schedinfo_test

import (
	"math"

	"github.com/mvrahden/go-test/internal/schedinfo"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SchedinfoTestSuite tests the scheduling-context diagnosis line: the
// percentile math on synthetic histograms and the summary's shape.
type SchedinfoTestSuite struct{}

func (s *SchedinfoTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SchedinfoTestSuite) TestHistogramPercentiles(t *gotest.T) {
	t.When("counts concentrate in one bucket", func(w *gotest.T) {
		w.It("reports that bucket's upper bound for both percentiles", func(it *gotest.T) {
			p50, p99, ok := schedinfo.ExportHistogramPercentiles(
				[]uint64{0, 100, 0}, []float64{0, 0.001, 0.01, 0.1}, 0.50, 0.99)
			gotest.True(it, ok)
			gotest.InDelta(it, 0.01, p50, 1e-9)
			gotest.InDelta(it, 0.01, p99, 1e-9)
		})
	})

	t.When("a heavy tail exists", func(w *gotest.T) {
		w.It("separates p50 from p99", func(it *gotest.T) {
			p50, p99, ok := schedinfo.ExportHistogramPercentiles(
				[]uint64{90, 0, 10}, []float64{0, 0.001, 0.01, 1.0}, 0.50, 0.99)
			gotest.True(it, ok)
			gotest.InDelta(it, 0.001, p50, 1e-9)
			gotest.InDelta(it, 1.0, p99, 1e-9)
		})
	})

	t.When("the top bucket is +Inf", func(w *gotest.T) {
		w.It("falls back to the finite lower bound instead of reporting infinity", func(it *gotest.T) {
			_, p99, ok := schedinfo.ExportHistogramPercentiles(
				[]uint64{1, 1}, []float64{0, 0.001, math.Inf(1)}, 0.50, 0.99)
			gotest.True(it, ok)
			gotest.InDelta(it, 0.001, p99, 1e-9)
		})
	})

	t.When("the histogram is empty", func(w *gotest.T) {
		w.It("reports not-ok rather than a fabricated zero", func(it *gotest.T) {
			_, _, ok := schedinfo.ExportHistogramPercentiles(nil, []float64{0}, 0.50, 0.99)
			gotest.False(it, ok)
		})
	})
}

func (s *SchedinfoTestSuite) TestSummary(t *gotest.T) {
	t.It("always carries the scheduler dimensions", func(it *gotest.T) {
		summary := schedinfo.Summary()
		gotest.Regexp(it, `^\(gomaxprocs=\d+ goroutines=\d+`, summary)
		gotest.Regexp(it, `\)$`, summary)
	})
}
