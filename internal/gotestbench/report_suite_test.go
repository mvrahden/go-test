package gotestbench_test

import (
	"encoding/json"

	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReportTestSuite tests the versioned `gotest bench --json` document: its
// schema stamp, its delta/gate presence rules, and the gate verdict math.
type ReportTestSuite struct{}

func (s *ReportTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ReportTestSuite) TestNewReport(t *gotest.T) {
	base := gotestbench.Baseline{Results: []gotestbench.Result{
		mkResult("pkg", "CacheTestSuite", "BenchmarkGetHit", []float64{100}),
	}}

	t.When("no comparison ran", func(w *gotest.T) {
		report := gotestbench.NewReport(base, nil, nil)

		w.It("stamps schema version 1", func(it *gotest.T) {
			gotest.Equal(it, 1, report.SchemaVersion)
		})

		w.It("omits deltas and gate from the JSON entirely", func(it *gotest.T) {
			data, err := gotestbench.MarshalReport(report)
			gotest.NoError(it, err)
			gotest.NotContains(it, string(data), `"deltas"`)
			gotest.NotContains(it, string(data), `"gate"`)
			gotest.Contains(it, string(data), `"baseline"`)
		})
	})

	t.When("a comparison and gate ran", func(w *gotest.T) {
		deltas := []gotestbench.Delta{
			{Key: "pkg CacheTestSuite/BenchmarkGetHit", OldNs: 100, NewNs: 112.3, PercentChange: 12.3, Significant: true},
		}
		gate := gotestbench.GateVerdict(deltas, 5)
		report := gotestbench.NewReport(base, deltas, &gate)

		w.It("round-trips deltas with their contract field names", func(it *gotest.T) {
			data, err := gotestbench.MarshalReport(report)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), `"percentChange": 12.3`)
			gotest.Contains(it, string(data), `"significant": true`)

			var parsed gotestbench.Report
			gotest.NoError(it, json.Unmarshal(data, &parsed))
			gotest.Equal(it, report.Deltas, parsed.Deltas)
			gotest.Equal(it, *report.Gate, *parsed.Gate)
		})
	})
}

func (s *ReportTestSuite) TestGateVerdict(t *gotest.T) {
	t.When("a significant regression exceeds the threshold", func(w *gotest.T) {
		verdict := gotestbench.GateVerdict([]gotestbench.Delta{
			{Key: "pkg A/B", PercentChange: 3, Significant: true},
			{Key: "pkg C/D", PercentChange: 12.3, Significant: true},
			{Key: "pkg E/F", PercentChange: 40, Significant: false},
		}, 5)

		w.It("reports the worst significant regression and breaches", func(it *gotest.T) {
			gotest.Equal(it, 12.3, verdict.WorstPct)
			gotest.Equal(it, "pkg C/D", verdict.WorstKey)
			gotest.True(it, verdict.Breached)
		})

		w.It("lists exactly the significant deltas above the threshold as breached", func(it *gotest.T) {
			gotest.Equal(it, []string{"pkg C/D"}, verdict.BreachedKeys)
		})

		w.It("never lets an insignificant delta drive the verdict", func(it *gotest.T) {
			gotest.NotEqual(it, "pkg E/F", verdict.WorstKey)
			gotest.NotContains(it, verdict.BreachedKeys, "pkg E/F")
		})
	})

	t.When("regressions stay under the threshold", func(w *gotest.T) {
		verdict := gotestbench.GateVerdict([]gotestbench.Delta{
			{Key: "pkg A/B", PercentChange: 3, Significant: true},
		}, 5)

		w.It("does not breach", func(it *gotest.T) {
			gotest.False(it, verdict.Breached)
			gotest.Equal(it, 3.0, verdict.WorstPct)
			gotest.Empty(it, verdict.BreachedKeys)
		})
	})

	t.When("only improvements exist", func(w *gotest.T) {
		verdict := gotestbench.GateVerdict([]gotestbench.Delta{
			{Key: "pkg A/B", PercentChange: -8.1, Significant: true},
		}, 5)

		w.It("reports a zero worst regression with no key", func(it *gotest.T) {
			gotest.Zero(it, verdict.WorstPct)
			gotest.Zero(it, verdict.WorstKey)
			gotest.False(it, verdict.Breached)
		})
	})
}
