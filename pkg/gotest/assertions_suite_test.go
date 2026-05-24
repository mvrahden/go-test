package gotest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var errSentinel = errors.New("sentinel error")

type myError struct {
	Code int
}

func (e *myError) Error() string { return fmt.Sprintf("myError: code=%d", e.Code) }

type AssertionsTestSuite struct{}

func (s *AssertionsTestSuite) TestFail(t *gotest.T) {
	t.It("always fails", func(it *gotest.T) {
		m := gotest.Record(func(r *gotest.R) { gotest.Fail(r) })
		gotest.True(it, m.Failed())
	})

	t.It("includes custom message", func(it *gotest.T) {
		m := gotest.Record(func(r *gotest.R) { gotest.Fail(r, "something went wrong: %d", 42) })
		gotest.True(it, m.Failed())
		gotest.Contains(it, m.Message(), "something went wrong: 42")
	})
}

func (s *AssertionsTestSuite) TestEqual(t *gotest.T) {
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
	})
}

func (s *AssertionsTestSuite) TestNotEqual(t *gotest.T) {
	t.When("values differ", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, 1, 2) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, "hello", "world") })
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

func (s *AssertionsTestSuite) TestTrue(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestFalse(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestZero(t *gotest.T) {
	t.When("value is the zero value", func(w *gotest.T) {
		w.It("passes for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, 0) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Zero(r, "") })
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
	})
}

func (s *AssertionsTestSuite) TestNotZero(t *gotest.T) {
	t.When("value is non-zero", func(w *gotest.T) {
		w.It("passes for int", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, 42) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotZero(r, "hello") })
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
	})
}

func (s *AssertionsTestSuite) TestEmpty(t *gotest.T) {
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

		// ---

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

		// ---

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

		// ---

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

		// ---

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

		// ---

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
}

func (s *AssertionsTestSuite) TestNotEmpty(t *gotest.T) {
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

		// ---

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

		// ---

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

		// ---

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

		// ---

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

		// ---

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
}

func (s *AssertionsTestSuite) TestNoError(t *gotest.T) {
	t.When("error is nil", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NoError(r, nil) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("error is non-nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NoError(r, errors.New("some error")) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestError(t *gotest.T) {
	t.When("error is non-nil", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Error(r, errors.New("some error")) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("error is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Error(r, nil) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestErrorIs(t *gotest.T) {
	t.When("error matches target", func(w *gotest.T) {
		w.It("passes for direct match", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorIs(r, errSentinel, errSentinel) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for wrapped error", func(it *gotest.T) {
			wrapped := fmt.Errorf("wrapped: %w", errSentinel)
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorIs(r, wrapped, errSentinel) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("error does not match", func(w *gotest.T) {
		w.It("fails for different error", func(it *gotest.T) {
			other := errors.New("other error")
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorIs(r, other, errSentinel) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil error", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorIs(r, nil, errSentinel) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestErrorAs(t *gotest.T) {
	t.When("error matches target type", func(w *gotest.T) {
		w.It("passes and returns matched error", func(it *gotest.T) {
			var got *myError
			m := gotest.Record(func(r *gotest.R) {
				got = gotest.ErrorAs[*myError](r, &myError{Code: 42})
			})
			gotest.False(it, m.Failed())
			gotest.Equal(it, 42, got.Code)
		})
		w.It("passes for wrapped error", func(it *gotest.T) {
			var got *myError
			m := gotest.Record(func(r *gotest.R) {
				got = gotest.ErrorAs[*myError](r, fmt.Errorf("wrapped: %w", &myError{Code: 7}))
			})
			gotest.False(it, m.Failed())
			gotest.Equal(it, 7, got.Code)
		})
	})

	t.When("error does not match type", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorAs[*myError](r, errors.New("plain error")) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestErrorContains(t *gotest.T) {
	t.When("error contains substring", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorContains(r, errors.New("file not found"), "not found") })
			gotest.False(it, m.Failed())
		})
	})

	t.When("error does not contain substring", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.ErrorContains(r, errors.New("file not found"), "connection refused")
			})
			gotest.True(it, m.Failed())
		})
	})

	t.When("error is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorContains(r, nil, "anything") })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestFailMessagePropagation(t *gotest.T) {
	t.When("custom message is provided", func(w *gotest.T) {
		w.It("includes message in failure output", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, 1, 2, "custom failure message") })
			gotest.True(it, m.Failed())
			gotest.NotEmpty(it, m.Message())
			gotest.Contains(it, m.Message(), "custom failure message")
		})
		w.It("supports format strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, false, "expected %s to be true", "value") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "expected value to be true")
		})
	})
}

func (s *AssertionsTestSuite) TestContains(t *gotest.T) {
	t.When("container includes element", func(w *gotest.T) {
		w.It("passes for string substring", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, "hello world", "world") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for slice element", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{1, 2, 3}, 2) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for map key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"a": 1, "b": 2}, "a") })
			gotest.False(it, m.Failed())
		})
	})

	t.When("container does not include element", func(w *gotest.T) {
		w.It("fails for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, "hello world", "xyz") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{1, 2, 3}, 99) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for map", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"a": 1}, "z") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("container is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, nil, "anything") })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestNotContains(t *gotest.T) {
	t.When("container does not include element", func(w *gotest.T) {
		w.It("passes for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, "hello world", "xyz") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int{1, 2, 3}, 99) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for map key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1}, "z") })
			gotest.False(it, m.Failed())
		})
	})

	t.When("container includes element", func(w *gotest.T) {
		w.It("fails for string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, "hello world", "world") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for slice", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int{1, 2, 3}, 2) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for map key", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1, "b": 2}, "a") })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestLen(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestElementsMatch(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestSubset(t *gotest.T) {
	t.When("subset is contained in list", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Subset(r, []int{1, 2, 3, 4, 5}, []int{2, 4}) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for empty subset", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Subset(r, []int{1, 2, 3}, []int{}) })
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

func (s *AssertionsTestSuite) TestGreater(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestGreaterOrEqual(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestLess(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestLessOrEqual(t *gotest.T) {
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

func (s *AssertionsTestSuite) TestRegexp(t *gotest.T) {
	t.When("string matches pattern", func(w *gotest.T) {
		w.It("passes for string pattern", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, `^\d+$`, "12345") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for compiled regexp", func(it *gotest.T) {
			re := regexp.MustCompile(`hello`)
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, re, "say hello world") })
			gotest.False(it, m.Failed())
		})
	})

	t.When("string does not match pattern", func(w *gotest.T) {
		w.It("fails for string pattern", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, `^\d+$`, "abc") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for compiled regexp", func(it *gotest.T) {
			re := regexp.MustCompile(`^hello$`)
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, re, "say hello world") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("pattern is invalid", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, `[invalid`, "test") })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestInDelta(t *gotest.T) {
	t.When("values are within delta", func(w *gotest.T) {
		w.It("passes for floats", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 3.14, 3.15, 0.02) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for ints", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 100, 101, 2.0) })
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
	})
}

func (s *AssertionsTestSuite) TestJSONEq(t *gotest.T) {
	t.When("JSON structures are equal", func(w *gotest.T) {
		w.It("passes for different key order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1,"b":2}`, `{"b":2,"a":1}`) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for []byte input", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, []byte(`{"x":10}`), []byte(`{"x":10}`)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for marshalable struct", func(it *gotest.T) {
			type S struct {
				A int `json:"a"`
			}
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, S{A: 5}, `{"a":5}`) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for io.Reader input", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.JSONEq(r, bytes.NewReader([]byte(`{"a":1,"b":2}`)), `{"b":2,"a":1}`)
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes for json.RawMessage input", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, json.RawMessage(`{"x":10}`), `{"x":10}`) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("JSON structures differ", func(w *gotest.T) {
		w.It("fails for different values", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1}`, `{"a":2}`) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for different keys", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1}`, `{"b":1}`) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is invalid JSON", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{not json}`, `{"a":1}`) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestTimeWithin(t *gotest.T) {
	t.When("times are within tolerance", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base.Add(50*time.Millisecond), 100*time.Millisecond)
			})
			gotest.False(it, m.Failed())
		})
	})

	t.When("times exceed tolerance", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base.Add(200*time.Millisecond), 100*time.Millisecond)
			})
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestTimeIsNow(t *gotest.T) {
	t.When("time is recent", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.TimeIsNow(r, time.Now(), time.Second) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("time is old", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.TimeIsNow(r, time.Now().Add(-time.Hour), time.Second) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestPanics(t *gotest.T) {
	t.When("function panics", func(w *gotest.T) {
		w.It("passes and returns recovered value", func(it *gotest.T) {
			var v any
			m := gotest.Record(func(r *gotest.R) {
				v = gotest.Panics(r, func() { panic("oh no") })
			})
			gotest.False(it, m.Failed())
			gotest.Equal(it, "oh no", v)
		})
	})

	t.When("function does not panic", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Panics(r, func() {}) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestEventually(t *gotest.T) {
	t.When("condition becomes true before timeout", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			count := 0
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 50*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					count++
					gotest.GreaterOrEqual(poll, count, 3)
				})
			})
			gotest.False(it, m.Failed())
		})
	})

	t.When("condition never becomes true", func(w *gotest.T) {
		w.It("fails after timeout", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 10*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, false)
				})
			})
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AssertionsTestSuite) TestConsistently(t *gotest.T) {
	t.When("condition stays true for duration", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 20*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, true)
				})
			})
			gotest.False(it, m.Failed())
		})
	})

	t.When("condition becomes false", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			count := 0
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 50*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					count++
					gotest.Less(poll, count, 3)
				})
			})
			gotest.True(it, m.Failed())
		})
	})
}
