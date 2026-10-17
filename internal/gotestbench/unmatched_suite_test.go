package gotestbench_test

import (
	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// UnmatchedTestSuite: a comparison says which benchmarks it could not
// compare, on either side, instead of silently judging the intersection.
type UnmatchedTestSuite struct{}

func (s *UnmatchedTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *UnmatchedTestSuite) TestKeysOnOneSideOnly(t *gotest.T) {
	old := gotestbench.Baseline{Results: []gotestbench.Result{
		{Package: "m/a", Suite: "S", Name: "Old"},
		{Package: "m/a", Suite: "S", Name: "Both"},
	}}
	cur := gotestbench.Baseline{Results: []gotestbench.Result{
		{Package: "m/a", Suite: "S", Name: "Both"},
		{Package: "m/b", Suite: "T", Name: "New"},
	}}
	missing, added := gotestbench.Unmatched(old, cur)
	t.When("the baseline and the run do not list the same benchmarks", func(w *gotest.T) {
		w.It("names the baseline's benchmarks the run did not produce", func(it *gotest.T) {
			gotest.Equal(it, []string{"m/a S/Old"}, missing)
		})
		w.It("names the run's benchmarks the baseline does not have", func(it *gotest.T) {
			gotest.Equal(it, []string{"m/b T/New"}, added)
		})
	})
}
