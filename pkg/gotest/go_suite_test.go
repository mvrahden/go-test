package gotest_test

import (
	"sync/atomic"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoTestSuite: gotest.Go starts a goroutine for any T, the TB-backed ones of
// benchmark and fuzz hooks included.
type GoTestSuite struct{}

func (s *GoTestSuite) TestTBBackedT(t *gotest.T) {
	t.When("Go is called with the TB-backed T a benchmark hook receives", func(w *gotest.T) {
		var waited atomic.Bool
		testing.Benchmark(func(b *testing.B) {
			gotest.Go(gotest.NewTFromTB(b), func() { waited.Store(true) })
			for b.Loop() {
			}
		})
		w.It("registers the wait as the benchmark's cleanup instead of panicking", func(it *gotest.T) {
			gotest.True(it, waited.Load())
		})
	})
}
