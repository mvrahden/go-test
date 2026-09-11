package gotest_test

import (
	"math"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// OrderingAssertionsTestSuite covers the ordered and numeric assertions.
type OrderingAssertionsTestSuite struct{}

func (s *OrderingAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *OrderingAssertionsTestSuite) TestGreater(t *gotest.T) {
	t.When("a is greater than b", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Greater(r, 5, 3) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for floats", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Greater(r, 3.14, 2.71) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("a is not greater", func(w *gotest.T) {
		w.It("fails when less", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Greater(r, 3, 5) })
			gotest.True(it, m.Failed())
		})
		w.It("fails when equal", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Greater(r, 4, 4) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *OrderingAssertionsTestSuite) TestGreaterOrEqual(t *gotest.T) {
	t.When("a is greater than or equal to b", func(w *gotest.T) {
		w.It("passes when greater", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.GreaterOrEqual(r, 5, 3) })
			gotest.False(it, m.Failed())
		})
		w.It("passes when equal", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.GreaterOrEqual(r, 4, 4) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for equal floats", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.GreaterOrEqual(r, 3.14, 3.14) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("a is less than b", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.GreaterOrEqual(r, 3, 5) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *OrderingAssertionsTestSuite) TestLess(t *gotest.T) {
	t.When("a is less than b", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Less(r, 3, 5) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("a is not less", func(w *gotest.T) {
		w.It("fails when greater", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Less(r, 5, 3) })
			gotest.True(it, m.Failed())
		})
		w.It("fails when equal", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Less(r, 4, 4) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *OrderingAssertionsTestSuite) TestLessOrEqual(t *gotest.T) {
	t.When("a is less than or equal to b", func(w *gotest.T) {
		w.It("passes when less", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.LessOrEqual(r, 3, 5) })
			gotest.False(it, m.Failed())
		})
		w.It("passes when equal", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.LessOrEqual(r, 4, 4) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("a is greater than b", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.LessOrEqual(r, 5, 3) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *OrderingAssertionsTestSuite) TestInDelta(t *gotest.T) {
	t.When("values are within delta", func(w *gotest.T) {
		w.It("passes for floats", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 3.14, 3.15, 0.02) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 100, 101, 2.0) })
			gotest.False(it, m.Failed())
		})
		w.It("passes at exact boundary", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 100, 102, 2.0) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for zero delta with equal values", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 5, 5, 0.0) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for unsigned ints where expected < actual", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, uint(3), uint(5), 3.0) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for int8 near boundary", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, int8(100), int8(-20), 121.0) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("values exceed delta", func(w *gotest.T) {
		w.It("fails for floats", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 3.14, 3.50, 0.02) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 100, 105, 2.0) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for int8 overflow that would mask delta", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, int8(100), int8(-50), 110) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for unsigned ints where expected < actual", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, uint(0), uint(5), 3.0) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("delta is negative", func(w *gotest.T) {
		w.It("always fails even for equal values", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 5, 5, -1.0) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("delta is zero", func(w *gotest.T) {
		w.It("fails for unequal values", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 5, 6, 0.0) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("values are NaN", func(w *gotest.T) {
		w.It("fails when expected is NaN", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, math.NaN(), 1.0, 100.0) })
			gotest.True(it, m.Failed())
		})
		w.It("fails when actual is NaN", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 1.0, math.NaN(), 100.0) })
			gotest.True(it, m.Failed())
		})
		w.It("fails when both are NaN", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, math.NaN(), math.NaN(), 100.0) })
			gotest.True(it, m.Failed())
		})
	})
}
