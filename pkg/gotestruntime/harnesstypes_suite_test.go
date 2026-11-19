package gotestruntime_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// HarnessTypesTestSuite tests the values generated benchmark and fuzz
// wrappers build through the runtime.
type HarnessTypesTestSuite struct{}

func (s *HarnessTypesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *HarnessTypesTestSuite) TestEachT(t *gotest.T) {
	each := gotestruntime.EachT(t.T())
	t.When("generated code wraps a testing value for a benchmark's hooks", func(w *gotest.T) {
		w.It("has no *testing.T behind it", func(it *gotest.T) { gotest.Zero(it, each.T()) })
		w.It("carries the value's name and context", func(it *gotest.T) {
			gotest.Equal(it, t.Name(), each.Name())
			gotest.Equal(it, t.T().Context(), each.Context()) //nolint:t-escape // the wrapped value's own context is the expectation
		})
	})
}

func (s *HarnessTypesTestSuite) TestBenchB(t *gotest.T) {
	var same bool
	testing.Benchmark(func(b *testing.B) { same = gotestruntime.BenchB(b).B() == b })
	t.When("generated code wraps a *testing.B", func(w *gotest.T) {
		w.It("the wrapper returns that B", func(it *gotest.T) { gotest.True(it, same) })
	})
}

func (s *HarnessTypesTestSuite) TestSeeds(t *gotest.T) {
	t.When("a seed has the wrong number of values", func(w *gotest.T) {
		w.It("names both counts", func(it *gotest.T) {
			gotest.ErrorContains(it, gotestruntime.SeedArity([]any{1, 2}, 3), "f.Add was given 2 values, but this fuzz target takes 3")
			gotest.NoError(it, gotestruntime.SeedArity([]any{1}, 1))
		})
	})
	t.When("a seed value has the wrong type", func(w *gotest.T) {
		w.It("names its position and both types", func(it *gotest.T) {
			_, err := gotestruntime.SeedArg[string]([]any{1, 2}, 1)
			gotest.ErrorContains(it, err, "value 2: f.Add was given int, but this fuzz target takes string")
			v, err := gotestruntime.SeedArg[int]([]any{1, 2}, 1)
			gotest.NoError(it, err)
			gotest.Equal(it, 2, v)
		})
	})
}

// The types generated code names are the user package's own.
var (
	_ func(*testing.F, func(*gotest.T), func(*gotest.T), *gotestruntime.FuzzTarget) *gotest.F = gotestruntime.FuzzF
	_ *gotest.FuzzTarget                                                                      = (*gotestruntime.FuzzTarget)(nil)
	_ gotest.FuzzEach                                                                         = gotestruntime.FuzzEach(nil)
)
