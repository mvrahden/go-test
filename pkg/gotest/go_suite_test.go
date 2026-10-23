package gotest_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/setupwaits"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoTestSuite: gotest.Go starts a goroutine for any T, the TB-backed ones of
// benchmark and fuzz hooks included, and registers its wait where the T's
// suite waits for it.
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

// inSuite runs body on a subtest of t standing in for a suite whose waits
// BeforeAll registers: bt is BeforeAll's T. It returns the suite's set.
func inSuite(t *gotest.T, body func(st *testing.T, bt *gotest.T)) *setupwaits.Set {
	var set *setupwaits.Set
	t.T().Run("suite", func(st *testing.T) { //nolint:suite-lifecycle // stands in for a suite's own T
		body(st, gotest.NewTWithContext(st, setupwaits.Open(st.Context(), st)))
		set = setupwaits.Close(st)
	})
	return set
}

func (s *GoTestSuite) TestSetupWaits(t *gotest.T) {
	// The goroutines return at once: a wait left to the T's cleanup ends
	// there, and the count below is short of it.
	t.When("Go is called with a T derived from BeforeAll's", func(w *gotest.T) {
		set := inSuite(w, func(_ *testing.T, bt *gotest.T) {
			gotest.Go(gotest.NewTWithDeadline(bt, time.Minute), func() {})
			bt.It("in an It", func(it *gotest.T) {
				gotest.Go(it, func() {})
				for row := range gotest.Each(it, []int{1}) {
					gotest.Go(row, func() {})
				}
			})
		})
		w.It("hands every wait to the suite's set, not to the cleanup of the T", func(it *gotest.T) {
			gotest.Len(it, set.Take(), 3)
		})
	})
	t.When("Go is called with a T wrapped around a hook's *testing.T", func(w *gotest.T) {
		set := inSuite(w, func(st *testing.T, _ *gotest.T) {
			gotest.Go(gotest.NewT(st), func() {})
			gotest.Go(gotest.NewTWithDeadline(st, time.Minute), func() {})
			gotest.NewT(st).When("in a When", func(ww *gotest.T) { gotest.Go(ww, func() {}) })
		})
		w.It("finds the suite's set through the *testing.T", func(it *gotest.T) {
			gotest.Len(it, set.Take(), 3)
		})
	})
}

func (s *GoTestSuite) TestLateRegistration(t *gotest.T) {
	t.When("Go is called with BeforeAll's T after its waits were drained", func(w *gotest.T) {
		var ended atomic.Bool
		inSuite(w, func(_ *testing.T, bt *gotest.T) {
			gotest.Empty(w, setupwaits.Close(bt.T()).Take()) // drained, and found nothing: closed
			gotest.Go(bt, func() { time.Sleep(10 * time.Millisecond); ended.Store(true) })
		})
		w.It("waits for the goroutine before it returns", func(it *gotest.T) {
			gotest.True(it, ended.Load())
		})
	})
}

func (s *GoTestSuite) TestGoexit(t *gotest.T) {
	var skipped, failed bool
	t.T().Run("skip", func(mt *testing.T) { //nolint:suite-lifecycle // the method under test is a subtest of this one
		defer func() { skipped, failed = mt.Skipped(), mt.Failed() }()
		gt := gotest.NewT(mt)
		gotest.Go(gt, func() { gt.Skipf("not here") })()
	})
	t.When("the goroutine skips the test", func(w *gotest.T) {
		w.It("the test is skipped at the wait, not failed", func(it *gotest.T) {
			gotest.True(it, skipped)
			gotest.False(it, failed)
		})
	})
}
