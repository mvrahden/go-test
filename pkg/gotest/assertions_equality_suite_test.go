package gotest_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EqualityAssertionsTestSuite covers equality, boolean and zero-value assertions, and how a
// failure message reaches the test.
type EqualityAssertionsTestSuite struct{}

func (s *EqualityAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *EqualityAssertionsTestSuite) TestFail(t *gotest.T) {
	t.When("called without message", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Fail(r) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("called with message", func(w *gotest.T) {
		w.It("includes the formatted message", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Fail(r, "something went wrong: %d", 42) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "something went wrong: 42")
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestFailMessagePropagation(t *gotest.T) {
	t.When("custom message is provided", func(w *gotest.T) {
		w.It("includes message in failure output", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, 1, 2, "custom failure message") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "custom failure message")
		})
		w.It("supports format strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, false, "expected %s to be true", "value") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "expected value to be true")
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestEqual(t *gotest.T) {
	t.When("values are deeply equal", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, 42, 42) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, "hello", "hello") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for slices", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, []int{1, 2, 3}, []int{1, 2, 3}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for structs", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, point{1, 2}, point{1, 2}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for maps regardless of key order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Equal(r, map[string]int{"a": 1, "b": 2}, map[string]int{"b": 2, "a": 1})
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil slices", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal[[]int](r, nil, nil) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("values differ", func(w *gotest.T) {
		w.It("fails for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, 1, 2) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, "hello", "world") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for slices", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, []int{1, 2}, []int{3, 4}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil slice vs empty slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, []int(nil), []int{}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for structs", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, point{1, 2}, point{3, 4}) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestNotEqual(t *gotest.T) {
	t.When("values differ", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, 1, 2) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, "hello", "world") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil slice vs empty slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, []int(nil), []int{}) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("values are the same", func(w *gotest.T) {
		w.It("fails for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, 42, 42) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, "same", "same") })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestTrue(t *gotest.T) {
	t.When("value is true", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, true) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("value is false", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, false) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestFalse(t *gotest.T) {
	t.When("value is false", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.False(r, false) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("value is true", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.False(r, true) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestZero(t *gotest.T) {
	t.When("value is the zero value", func(w *gotest.T) {
		w.It("passes for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, 0) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, "") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for bool", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, false) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil pointer", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero[*int](r, nil) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, point{}) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("value is non-zero", func(w *gotest.T) {
		w.It("fails for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, 42) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, "hello") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for bool", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, true) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for non-nil pointer", func(it *gotest.T) {
			n := 1
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, &n) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, point{1, 2}) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestSame(t *gotest.T) {
	type node struct{ v int }
	t.When("both arguments point at one value", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			n := &node{1}
			m := gotest.Record(func(r *gotest.R) { gotest.Same(r, n, n) })
			gotest.False(it, m.Failed())
		})
	})
	t.When("they point at equal but distinct values", func(w *gotest.T) {
		w.It("fails, naming both addresses", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Same(r, &node{1}, &node{1}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "Same failed")
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestNotSame(t *gotest.T) {
	type node struct{ v int }
	t.When("they point at equal but distinct values", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotSame(r, &node{1}, &node{1}) })
			gotest.False(it, m.Failed())
		})
	})
	t.When("both arguments point at one value", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			n := &node{1}
			m := gotest.Record(func(r *gotest.R) { gotest.NotSame(r, n, n) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "NotSame failed")
		})
	})
}

func (s *EqualityAssertionsTestSuite) TestNotZero(t *gotest.T) {
	t.When("value is non-zero", func(w *gotest.T) {
		w.It("passes for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, 42) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, "hello") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for bool", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, true) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for non-nil pointer", func(it *gotest.T) {
			n := 1
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, &n) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, point{1, 2}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for float64", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, 3.14) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for non-nil channel", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, make(chan int)) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("value is zero", func(w *gotest.T) {
		w.It("fails for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, 0) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, "") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for bool", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, false) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil pointer", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero[*int](r, nil) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, point{}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for float64", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, 0.0) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil channel", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero[chan int](r, nil) })
			gotest.True(it, m.Failed())
		})
	})
}
