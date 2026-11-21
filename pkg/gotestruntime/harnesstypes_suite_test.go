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

// FuzzF takes the hooks and target generated code hands it.
var _ func(*testing.F, func(*gotest.T), func(*gotest.T), *gotestruntime.FuzzTarget) *gotest.F = gotestruntime.FuzzF

func (s *HarnessTypesTestSuite) TestFuzzCodecs(t *gotest.T) {
	t.When("a number travels as a leaf", func(w *gotest.T) {
		w.It("decodes to the value it was encoded from", func(it *gotest.T) {
			gotest.Equal(it, int16(-7), gotestruntime.FuzzLeaf[int16](gotestruntime.FuzzLeafBytes(int16(-7))))
			gotest.Equal(it, uint64(1<<40), gotestruntime.FuzzLeaf[uint64](gotestruntime.FuzzLeafBytes(uint64(1<<40))))
			gotest.Equal(it, 2.5, gotestruntime.FuzzLeaf[float64](gotestruntime.FuzzLeafBytes(2.5)))
			gotest.Equal(it, float32(-1.25), gotestruntime.FuzzLeaf[float32](gotestruntime.FuzzLeafBytes(float32(-1.25))))
			gotest.Equal(it, byte(200), gotestruntime.FuzzLeaf[byte](gotestruntime.FuzzLeafBytes(byte(200))))
		})
		w.It("is as wide as its type", func(it *gotest.T) {
			gotest.Len(it, gotestruntime.FuzzLeafBytes(int8(1)), 1)
			gotest.Len(it, gotestruntime.FuzzLeafBytes(uint32(1)), 4)
			gotest.Len(it, gotestruntime.FuzzLeafBytes(1), 8)
		})
	})
	t.When("a []byte leaf is empty", func(w *gotest.T) {
		w.It("becomes nil", func(it *gotest.T) {
			gotest.Nil(it, gotestruntime.FuzzBytes([]byte{}))
			gotest.Equal(it, []byte("x"), gotestruntime.FuzzBytes([]byte("x")))
		})
	})
	t.When("a value is written and read back", func(w *gotest.T) {
		w.It("reads what was written", func(it *gotest.T) {
			fw := gotestruntime.NewFuzzWriter()
			fw.String("hi")
			fw.Int32(-3)
			fr := gotestruntime.NewFuzzReader(fw.Out())
			gotest.Equal(it, "hi", fr.String())
			gotest.Equal(it, int32(-3), fr.Int32())
		})
	})
}
