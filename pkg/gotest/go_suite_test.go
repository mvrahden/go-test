package gotest_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoTestSuite: gotest.Go starts a goroutine for any T, the TB-backed ones of
// benchmark and fuzz hooks included.
type GoTestSuite struct{}

func (s *GoTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

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

func (s *GoTestSuite) TestDeadlineTKeepsGoWaits(t *gotest.T) {
	t.When("Go is called with a NewTWithDeadline T derived from a T whose waits are kept", func(w *gotest.T) {
		var kept []func()
		base := gotest.NewTWithGoWaits(w.T(), w.Context(), func(wait func()) { kept = append(kept, wait) })
		release := make(chan struct{})
		gotest.Go(gotest.NewTWithDeadline(base, time.Minute), func() { <-release })
		w.It("hands the wait to the base's register, not to the test's cleanup", func(it *gotest.T) {
			gotest.Len(it, kept, 1)
		})
		close(release)
		for _, wait := range kept {
			wait()
		}
	})
}
