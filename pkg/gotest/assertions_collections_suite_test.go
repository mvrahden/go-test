package gotest_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CollectionAssertionsTestSuite covers emptiness, nil, length and membership over strings, slices,
// arrays and maps, with the type guards each assertion enforces.
type CollectionAssertionsTestSuite struct{}

func (s *CollectionAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CollectionAssertionsTestSuite) TestEmpty(t *gotest.T) {
	t.When("input is nil", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, nil) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("input is a slice", func(w *gotest.T) {
		w.It("passes for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, []int{}) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, []int{1, 2, 3}) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a map", func(w *gotest.T) {
		w.It("passes for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, map[string]int{}) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, map[string]int{"a": 1}) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a string", func(w *gotest.T) {
		w.It("passes for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, "") })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, "hello") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a channel", func(w *gotest.T) {
		w.It("passes for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, make(chan int)) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for non-empty", func(it *gotest.T) {
			ch := make(chan int, 1)
			ch <- 42
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, ch) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a pointer", func(w *gotest.T) {
		w.It("passes for nil pointer", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, (*[]int)(nil)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for single indirection to empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, &[]int{}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for double indirection to empty", func(it *gotest.T) {
			inner := &[]int{}
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, &inner) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for triple indirection to empty", func(it *gotest.T) {
			s := []int{}
			p1 := &s
			p2 := &p1
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, &p2) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for single indirection to non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, &[]int{1, 2, 3}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for double indirection to non-empty", func(it *gotest.T) {
			inner := &[]int{1, 2, 3}
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, &inner) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for triple indirection to non-empty", func(it *gotest.T) {
			s := []int{1, 2, 3}
			p1 := &s
			p2 := &p1
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, &p2) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an array", func(w *gotest.T) {
		w.It("passes for zero-length array", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, [0]int{}) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for non-zero-length array", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, [3]int{1, 2, 3}) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an integer", func(w *gotest.T) {
		w.It("fails with type guard", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, 42) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "cannot be empty")
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestNotEmpty(t *gotest.T) {
	t.When("input is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, nil) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a slice", func(w *gotest.T) {
		w.It("passes for non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, []int{1, 2, 3}) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, []int{}) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a map", func(w *gotest.T) {
		w.It("passes for non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, map[string]int{"a": 1}) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, map[string]int{}) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a string", func(w *gotest.T) {
		w.It("passes for non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, "hello") })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, "") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a channel", func(w *gotest.T) {
		w.It("passes for non-empty", func(it *gotest.T) {
			ch := make(chan int, 1)
			ch <- 42
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, ch) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, make(chan int)) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a pointer", func(w *gotest.T) {
		w.It("passes for single indirection to non-empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, &[]int{1, 2, 3}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for double indirection to non-empty", func(it *gotest.T) {
			inner := &[]int{1, 2, 3}
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, &inner) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for triple indirection to non-empty", func(it *gotest.T) {
			s := []int{1, 2, 3}
			p1 := &s
			p2 := &p1
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, &p2) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for nil pointer", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, (*[]int)(nil)) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for single indirection to empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, &[]int{}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for double indirection to empty", func(it *gotest.T) {
			inner := &[]int{}
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, &inner) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for triple indirection to empty", func(it *gotest.T) {
			s := []int{}
			p1 := &s
			p2 := &p1
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, &p2) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an array", func(w *gotest.T) {
		w.It("passes for non-zero-length array", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, [3]int{1, 2, 3}) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for zero-length array", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, [0]int{}) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an integer", func(w *gotest.T) {
		w.It("fails with type guard", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEmpty(r, 42) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "cannot be empty")
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestNil(t *gotest.T) {
	t.When("value is nil", func(w *gotest.T) {
		w.It("passes for untyped nil", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, nil) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, []int(nil)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, map[string]int(nil)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil func", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, (func())(nil)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil pointer", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, (*int)(nil)) }) //nolint:assertion-simplify // testing guard behavior
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil channel", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, (chan int)(nil)) }) //nolint:assertion-simplify // testing guard behavior
			gotest.False(it, m.Failed())
		})
	})

	t.When("value is not nil", func(w *gotest.T) {
		w.It("fails for non-nil slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, []int{}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for non-nil map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, map[string]int{}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for non-nil func", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, func() {}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for non-nil pointer", func(it *gotest.T) {
			n := 1
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, &n) }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
		})
		w.It("fails for non-nil channel", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, make(chan int)) }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
		})
	})

	t.When("value is not nilable", func(w *gotest.T) {
		w.It("fails with type guard for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, 42) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
		w.It("fails with type guard for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, "hello") }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
		w.It("fails with type guard for bool", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, true) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
		w.It("fails with type guard for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Nil(r, point{}) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestNotNil(t *gotest.T) {
	t.When("value is not nil", func(w *gotest.T) {
		w.It("passes for non-nil slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, []int{}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for non-nil map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, map[string]int{}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for non-nil func", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, func() {}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for non-nil pointer", func(it *gotest.T) {
			n := 1
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, &n) }) //nolint:assertion-simplify // testing guard behavior
			gotest.False(it, m.Failed())
		})
		w.It("passes for non-nil channel", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, make(chan int)) }) //nolint:assertion-simplify // testing guard behavior
			gotest.False(it, m.Failed())
		})
	})

	t.When("value is nil", func(w *gotest.T) {
		w.It("fails for untyped nil", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, nil) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, []int(nil)) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, map[string]int(nil)) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil func", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, (func())(nil)) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil pointer", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, (*int)(nil)) }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil channel", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, (chan int)(nil)) }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
		})
	})

	t.When("value is not nilable", func(w *gotest.T) {
		w.It("fails with type guard for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, 42) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
		w.It("fails with type guard for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, "hello") }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
		w.It("fails with type guard for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotNil(r, point{}) }) //nolint:assertion-type-guard // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not nilable")
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestContains(t *gotest.T) {
	t.When("container is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, nil, "anything") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a string", func(w *gotest.T) {
		w.It("passes for matching substring", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, "hello world", "world") })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for non-matching substring", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, "hello world", "xyz") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for non-string element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, "hello world", 42) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a slice", func(w *gotest.T) {
		w.It("passes for present element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{1, 2, 3}, 2) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for absent element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{1, 2, 3}, 99) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a map", func(w *gotest.T) {
		w.It("passes for existing key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"a": 1, "b": 2}, "a") })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for missing key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"a": 1}, "z") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an array", func(w *gotest.T) {
		w.It("passes for present element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, [3]int{1, 2, 3}, 2) })
			gotest.False(it, m.Failed())
		})

		// --- fails

		w.It("fails for absent element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, [3]int{1, 2, 3}, 99) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an unsupported type", func(w *gotest.T) {
		w.It("fails with type error for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, 42, 2) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not a string, slice, array, or map")
		})
		w.It("fails with type error for struct", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, struct{ X int }{1}, 1) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not a string, slice, array, or map")
		})
		w.It("fails with type error for map with mismatched key type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"a": 1}, 42) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "does not contain")
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestNotContains(t *gotest.T) {
	t.When("container is nil", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, nil, "anything") })
			gotest.False(it, m.Failed())
		})
	})

	t.When("input is a string", func(w *gotest.T) {
		w.It("passes for non-matching substring", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, "hello world", "xyz") })
			gotest.False(it, m.Failed())
		})
		// --- fails
		w.It("fails for matching substring", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, "hello world", "world") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a slice", func(w *gotest.T) {
		w.It("passes for absent element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int{1, 2, 3}, 99) })
			gotest.False(it, m.Failed())
		})
		// --- fails
		w.It("fails for present element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int{1, 2, 3}, 2) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is a map", func(w *gotest.T) {
		w.It("passes for missing key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1}, "z") })
			gotest.False(it, m.Failed())
		})
		// --- fails
		w.It("fails for existing key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1, "b": 2}, "a") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an array", func(w *gotest.T) {
		w.It("passes for absent element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, [3]int{1, 2, 3}, 99) })
			gotest.False(it, m.Failed())
		})
		// --- fails
		w.It("fails for present element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, [3]int{1, 2, 3}, 2) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is an unsupported type", func(w *gotest.T) {
		w.It("fails with type error for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, 42, 2) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "not a string, slice, array, or map")
		})
		w.It("passes without panic for map with mismatched key type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1}, 42) })
			gotest.False(it, m.Failed())
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestLen(t *gotest.T) {
	t.When("length matches", func(w *gotest.T) {
		w.It("passes for slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, []int{1, 2, 3}, 3) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, "hello", 5) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, map[string]int{"a": 1, "b": 2}, 2) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for array", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, [3]int{1, 2, 3}, 3) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for channel", func(it *gotest.T) {
			ch := make(chan int, 3)
			ch <- 1
			ch <- 2
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, ch, 2) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, []int(nil)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nil map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Empty(r, map[string]int(nil)) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("length does not match", func(w *gotest.T) {
		w.It("fails for slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, []int{1, 2, 3}, 5) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, "hello", 99) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, map[string]int{"a": 1}, 5) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for negative expected length", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, []int{1, 2}, -1) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("object has no length", func(w *gotest.T) {
		w.It("fails for nil", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, nil, 0) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for invalid type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Len(r, 42, 1) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestElementsMatch(t *gotest.T) {
	t.When("elements match regardless of order", func(w *gotest.T) {
		w.It("passes for different order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch(r, []int{1, 2, 3}, []int{3, 1, 2}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for same order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch(r, []string{"a", "b"}, []string{"a", "b"}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for both empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch(r, []int{}, []int{}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for both nil", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch[int](r, nil, nil) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("elements differ", func(w *gotest.T) {
		w.It("fails for different elements", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch(r, []int{1, 2, 3}, []int{1, 2, 99}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for different lengths", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch(r, []int{1, 2}, []int{1, 2, 3}) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for same elements different counts", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ElementsMatch(r, []int{1, 1, 2}, []int{1, 2, 2}) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *CollectionAssertionsTestSuite) TestSubset(t *gotest.T) {
	t.When("subset is contained in list", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Subset(r, []int{1, 2, 3, 4, 5}, []int{2, 4}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for empty subset", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Subset(r, []int{1, 2, 3}, []int{}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for both empty", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Subset(r, []int{}, []int{}) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("subset has missing elements", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Subset(r, []int{1, 2, 3}, []int{2, 99}) })
			gotest.True(it, m.Failed())
		})
	})
}
