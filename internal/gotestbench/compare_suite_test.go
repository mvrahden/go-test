package gotestbench_test

import (
	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CompareTestSuite tests Welch's t-test significance comparison between two
// baselines.
type CompareTestSuite struct{}

func (s *CompareTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func mkResult(pkg, suite, name string, ns []float64) gotestbench.Result {
	samples := make([]gotestbench.Sample, len(ns))
	for i, v := range ns {
		samples[i] = gotestbench.Sample{Iterations: 1000, NsPerOp: v}
	}
	return gotestbench.Result{Package: pkg, Suite: suite, Name: name, Samples: samples}
}

func (s *CompareTestSuite) TestCompare(t *gotest.T) {
	t.When("both sides have >=4 samples from the same distribution", func(w *gotest.T) {
		w.It("is not significant", func(it *gotest.T) {
			old := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{98, 102, 99, 101, 100, 103}),
			}}
			new_ := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{101, 97, 100, 102, 99, 100}),
			}}

			deltas := gotestbench.Compare(old, new_)
			gotest.Len(it, deltas, 1)
			d := deltas[0]
			gotest.Equal(it, "pkg FooTestSuite/BenchmarkParse", d.Key)
			gotest.False(it, d.InsufficientSample)
			gotest.False(it, d.Significant)
		})
	})

	t.When("the new side is shifted +30% with low variance across 6 samples", func(w *gotest.T) {
		w.It("is significant with PercentChange near 30", func(it *gotest.T) {
			old := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{99, 101, 100, 98, 102, 100}),
			}}
			new_ := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{128.7, 131.3, 130.0, 127.4, 132.6, 130.0}),
			}}

			deltas := gotestbench.Compare(old, new_)
			gotest.Len(it, deltas, 1)
			d := deltas[0]
			gotest.False(it, d.InsufficientSample)
			gotest.True(it, d.Significant)
			gotest.InDelta(it, 30.0, d.PercentChange, 2.0)
		})
	})

	t.When("either side has fewer than 4 samples", func(w *gotest.T) {
		w.It("falls back to the 20%% heuristic and marks InsufficientSample", func(it *gotest.T) {
			old := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{100}),
			}}
			bigShift := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{131}),
			}}
			smallShift := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkParse", []float64{105}),
			}}

			dBig := gotestbench.Compare(old, bigShift)
			gotest.Len(it, dBig, 1)
			gotest.True(it, dBig[0].InsufficientSample)
			gotest.True(it, dBig[0].Significant)
			gotest.InDelta(it, 31.0, dBig[0].PercentChange, 0.5)

			dSmall := gotestbench.Compare(old, smallShift)
			gotest.Len(it, dSmall, 1)
			gotest.True(it, dSmall[0].InsufficientSample)
			gotest.False(it, dSmall[0].Significant)
		})
	})

	t.When("a benchmark exists only in one baseline", func(w *gotest.T) {
		w.It("is omitted from the deltas", func(it *gotest.T) {
			old := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkOld", []float64{100, 100, 100, 100}),
			}}
			new_ := gotestbench.Baseline{Results: []gotestbench.Result{
				mkResult("pkg", "FooTestSuite", "BenchmarkNew", []float64{100, 100, 100, 100}),
			}}

			deltas := gotestbench.Compare(old, new_)
			gotest.Empty(it, deltas)
		})
	})
}

func (s *CompareTestSuite) TestWorstRegression(t *gotest.T) {
	t.When("multiple deltas include significant and insignificant regressions", func(w *gotest.T) {
		w.It("returns the max significant positive PercentChange", func(it *gotest.T) {
			deltas := []gotestbench.Delta{
				{Key: "a", PercentChange: 5, Significant: false},
				{Key: "b", PercentChange: 40, Significant: true},
				{Key: "c", PercentChange: 15, Significant: true},
				{Key: "d", PercentChange: -50, Significant: true},
			}
			gotest.InDelta(it, 40.0, gotestbench.WorstRegression(deltas), 0.001)
		})
	})

	t.When("no deltas are significant", func(w *gotest.T) {
		w.It("returns 0", func(it *gotest.T) {
			deltas := []gotestbench.Delta{
				{Key: "a", PercentChange: 90, Significant: false},
			}
			gotest.Equal(it, 0.0, gotestbench.WorstRegression(deltas))
		})
	})
}
