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
		m := newMock()
		gotest.Fail(m)
		gotest.True(it, m.failed)
	})

	t.It("includes custom message", func(it *gotest.T) {
		m := newMock()
		gotest.Fail(m, "something went wrong: %d", 42)
		gotest.True(it, m.failed)
		gotest.Contains(it, m.message, "something went wrong: 42")
	})
}

func (s *AssertionsTestSuite) TestEqual(t *gotest.T) {
	t.When("values are deeply equal", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, 42, 42)
			gotest.False(it, m.failed)
		})
		w.It("passes for strings", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, "hello", "hello")
			gotest.False(it, m.failed)
		})
		w.It("passes for slices", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, []int{1, 2, 3}, []int{1, 2, 3})
			gotest.False(it, m.failed)
		})
	})

	t.When("values differ", func(w *gotest.T) {
		w.It("fails for ints", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, 1, 2)
			gotest.True(it, m.failed)
		})
		w.It("fails for strings", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, "hello", "world")
			gotest.True(it, m.failed)
		})
		w.It("fails for slices", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, []int{1, 2}, []int{3, 4})
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestNotEqual(t *gotest.T) {
	t.When("values differ", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := newMock()
			gotest.NotEqual(m, 1, 2)
			gotest.False(it, m.failed)
		})
		w.It("passes for strings", func(it *gotest.T) {
			m := newMock()
			gotest.NotEqual(m, "hello", "world")
			gotest.False(it, m.failed)
		})
	})

	t.When("values are the same", func(w *gotest.T) {
		w.It("fails for ints", func(it *gotest.T) {
			m := newMock()
			gotest.NotEqual(m, 42, 42)
			gotest.True(it, m.failed)
		})
		w.It("fails for strings", func(it *gotest.T) {
			m := newMock()
			gotest.NotEqual(m, "same", "same")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestTrue(t *gotest.T) {
	t.When("value is true", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.True(m, true)
			gotest.False(it, m.failed)
		})
	})

	t.When("value is false", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.True(m, false)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestFalse(t *gotest.T) {
	t.When("value is false", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.False(m, false)
			gotest.False(it, m.failed)
		})
	})

	t.When("value is true", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.False(m, true)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestZero(t *gotest.T) {
	t.When("value is the zero value", func(w *gotest.T) {
		w.It("passes for int", func(it *gotest.T) {
			m := newMock()
			gotest.Zero(m, 0)
			gotest.False(it, m.failed)
		})
		w.It("passes for string", func(it *gotest.T) {
			m := newMock()
			gotest.Zero(m, "")
			gotest.False(it, m.failed)
		})
	})

	t.When("value is non-zero", func(w *gotest.T) {
		w.It("fails for int", func(it *gotest.T) {
			m := newMock()
			gotest.Zero(m, 42)
			gotest.True(it, m.failed)
		})
		w.It("fails for string", func(it *gotest.T) {
			m := newMock()
			gotest.Zero(m, "hello")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestNotZero(t *gotest.T) {
	t.When("value is non-zero", func(w *gotest.T) {
		w.It("passes for int", func(it *gotest.T) {
			m := newMock()
			gotest.NotZero(m, 42)
			gotest.False(it, m.failed)
		})
		w.It("passes for string", func(it *gotest.T) {
			m := newMock()
			gotest.NotZero(m, "hello")
			gotest.False(it, m.failed)
		})
	})

	t.When("value is zero", func(w *gotest.T) {
		w.It("fails for int", func(it *gotest.T) {
			m := newMock()
			gotest.NotZero(m, 0)
			gotest.True(it, m.failed)
		})
		w.It("fails for string", func(it *gotest.T) {
			m := newMock()
			gotest.NotZero(m, "")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestEmpty(t *gotest.T) {
	t.When("object is empty", func(w *gotest.T) {
		w.It("passes for nil", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, nil)
			gotest.False(it, m.failed)
		})
		w.It("passes for empty slice", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, []int{})
			gotest.False(it, m.failed)
		})
		w.It("passes for empty string", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, "")
			gotest.False(it, m.failed)
		})
		w.It("passes for empty map", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, map[string]int{})
			gotest.False(it, m.failed)
		})
	})

	t.When("object is not empty", func(w *gotest.T) {
		w.It("fails for non-empty slice", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, []int{1, 2, 3})
			gotest.True(it, m.failed)
		})
		w.It("fails for non-empty string", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, "hello")
			gotest.True(it, m.failed)
		})
		w.It("fails for non-empty map", func(it *gotest.T) {
			m := newMock()
			gotest.Empty(m, map[string]int{"a": 1})
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestNotEmpty(t *gotest.T) {
	t.When("object is not empty", func(w *gotest.T) {
		w.It("passes for non-empty slice", func(it *gotest.T) {
			m := newMock()
			gotest.NotEmpty(m, []int{1, 2, 3})
			gotest.False(it, m.failed)
		})
		w.It("passes for non-empty string", func(it *gotest.T) {
			m := newMock()
			gotest.NotEmpty(m, "hello")
			gotest.False(it, m.failed)
		})
	})

	t.When("object is empty", func(w *gotest.T) {
		w.It("fails for nil", func(it *gotest.T) {
			m := newMock()
			gotest.NotEmpty(m, nil)
			gotest.True(it, m.failed)
		})
		w.It("fails for empty slice", func(it *gotest.T) {
			m := newMock()
			gotest.NotEmpty(m, []int{})
			gotest.True(it, m.failed)
		})
		w.It("fails for empty string", func(it *gotest.T) {
			m := newMock()
			gotest.NotEmpty(m, "")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestNoError(t *gotest.T) {
	t.When("error is nil", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.NoError(m, nil)
			gotest.False(it, m.failed)
		})
	})

	t.When("error is non-nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.NoError(m, errors.New("some error"))
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestError(t *gotest.T) {
	t.When("error is non-nil", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.Error(m, errors.New("some error"))
			gotest.False(it, m.failed)
		})
	})

	t.When("error is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.Error(m, nil)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestErrorIs(t *gotest.T) {
	t.When("error matches target", func(w *gotest.T) {
		w.It("passes for direct match", func(it *gotest.T) {
			m := newMock()
			gotest.ErrorIs(m, errSentinel, errSentinel)
			gotest.False(it, m.failed)
		})
		w.It("passes for wrapped error", func(it *gotest.T) {
			m := newMock()
			wrapped := fmt.Errorf("wrapped: %w", errSentinel)
			gotest.ErrorIs(m, wrapped, errSentinel)
			gotest.False(it, m.failed)
		})
	})

	t.When("error does not match", func(w *gotest.T) {
		w.It("fails for different error", func(it *gotest.T) {
			m := newMock()
			other := errors.New("other error")
			gotest.ErrorIs(m, other, errSentinel)
			gotest.True(it, m.failed)
		})
		w.It("fails for nil error", func(it *gotest.T) {
			m := newMock()
			gotest.ErrorIs(m, nil, errSentinel)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestErrorAs(t *gotest.T) {
	t.When("error matches target type", func(w *gotest.T) {
		w.It("passes and returns matched error", func(it *gotest.T) {
			m := newMock()
			original := &myError{Code: 42}
			got := gotest.ErrorAs[*myError](m, original)
			gotest.False(it, m.failed)
			gotest.Equal(it, 42, got.Code)
		})
		w.It("passes for wrapped error", func(it *gotest.T) {
			m := newMock()
			original := &myError{Code: 7}
			wrapped := fmt.Errorf("wrapped: %w", original)
			got := gotest.ErrorAs[*myError](m, wrapped)
			gotest.False(it, m.failed)
			gotest.Equal(it, 7, got.Code)
		})
	})

	t.When("error does not match type", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			other := errors.New("plain error")
			gotest.ErrorAs[*myError](m, other)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestErrorContains(t *gotest.T) {
	t.When("error contains substring", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.ErrorContains(m, errors.New("file not found"), "not found")
			gotest.False(it, m.failed)
		})
	})

	t.When("error does not contain substring", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.ErrorContains(m, errors.New("file not found"), "connection refused")
			gotest.True(it, m.failed)
		})
	})

	t.When("error is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.ErrorContains(m, nil, "anything")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestFailMessagePropagation(t *gotest.T) {
	t.When("custom message is provided", func(w *gotest.T) {
		w.It("includes message in failure output", func(it *gotest.T) {
			m := newMock()
			gotest.Equal(m, 1, 2, "custom failure message")
			gotest.True(it, m.failed)
			gotest.NotEmpty(it, m.message)
			gotest.Contains(it, m.message, "custom failure message")
		})
		w.It("supports format strings", func(it *gotest.T) {
			m := newMock()
			gotest.True(m, false, "expected %s to be true", "value")
			gotest.True(it, m.failed)
			gotest.Contains(it, m.message, "expected value to be true")
		})
	})
}

func (s *AssertionsTestSuite) TestContains(t *gotest.T) {
	t.When("container includes element", func(w *gotest.T) {
		w.It("passes for string substring", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, "hello world", "world")
			gotest.False(it, m.failed)
		})
		w.It("passes for slice element", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, []int{1, 2, 3}, 2)
			gotest.False(it, m.failed)
		})
		w.It("passes for map key", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, map[string]int{"a": 1, "b": 2}, "a")
			gotest.False(it, m.failed)
		})
	})

	t.When("container does not include element", func(w *gotest.T) {
		w.It("fails for string", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, "hello world", "xyz")
			gotest.True(it, m.failed)
		})
		w.It("fails for slice", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, []int{1, 2, 3}, 99)
			gotest.True(it, m.failed)
		})
		w.It("fails for map", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, map[string]int{"a": 1}, "z")
			gotest.True(it, m.failed)
		})
	})

	t.When("container is nil", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.Contains(m, nil, "anything")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestNotContains(t *gotest.T) {
	t.When("container does not include element", func(w *gotest.T) {
		w.It("passes for string", func(it *gotest.T) {
			m := newMock()
			gotest.NotContains(m, "hello world", "xyz")
			gotest.False(it, m.failed)
		})
		w.It("passes for slice", func(it *gotest.T) {
			m := newMock()
			gotest.NotContains(m, []int{1, 2, 3}, 99)
			gotest.False(it, m.failed)
		})
		w.It("passes for map key", func(it *gotest.T) {
			m := newMock()
			gotest.NotContains(m, map[string]int{"a": 1}, "z")
			gotest.False(it, m.failed)
		})
	})

	t.When("container includes element", func(w *gotest.T) {
		w.It("fails for string", func(it *gotest.T) {
			m := newMock()
			gotest.NotContains(m, "hello world", "world")
			gotest.True(it, m.failed)
		})
		w.It("fails for slice", func(it *gotest.T) {
			m := newMock()
			gotest.NotContains(m, []int{1, 2, 3}, 2)
			gotest.True(it, m.failed)
		})
		w.It("fails for map key", func(it *gotest.T) {
			m := newMock()
			gotest.NotContains(m, map[string]int{"a": 1, "b": 2}, "a")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestLen(t *gotest.T) {
	t.When("length matches", func(w *gotest.T) {
		w.It("passes for slice", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, []int{1, 2, 3}, 3)
			gotest.False(it, m.failed)
		})
		w.It("passes for string", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, "hello", 5)
			gotest.False(it, m.failed)
		})
		w.It("passes for map", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, map[string]int{"a": 1, "b": 2}, 2)
			gotest.False(it, m.failed)
		})
	})

	t.When("length does not match", func(w *gotest.T) {
		w.It("fails for slice", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, []int{1, 2, 3}, 5)
			gotest.True(it, m.failed)
		})
		w.It("fails for string", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, "hello", 99)
			gotest.True(it, m.failed)
		})
	})

	t.When("object has no length", func(w *gotest.T) {
		w.It("fails for nil", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, nil, 0)
			gotest.True(it, m.failed)
		})
		w.It("fails for invalid type", func(it *gotest.T) {
			m := newMock()
			gotest.Len(m, 42, 1)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestElementsMatch(t *gotest.T) {
	t.When("elements match regardless of order", func(w *gotest.T) {
		w.It("passes for different order", func(it *gotest.T) {
			m := newMock()
			gotest.ElementsMatch(m, []int{1, 2, 3}, []int{3, 1, 2})
			gotest.False(it, m.failed)
		})
		w.It("passes for same order", func(it *gotest.T) {
			m := newMock()
			gotest.ElementsMatch(m, []string{"a", "b"}, []string{"a", "b"})
			gotest.False(it, m.failed)
		})
		w.It("passes for both empty", func(it *gotest.T) {
			m := newMock()
			gotest.ElementsMatch(m, []int{}, []int{})
			gotest.False(it, m.failed)
		})
	})

	t.When("elements differ", func(w *gotest.T) {
		w.It("fails for different elements", func(it *gotest.T) {
			m := newMock()
			gotest.ElementsMatch(m, []int{1, 2, 3}, []int{1, 2, 99})
			gotest.True(it, m.failed)
		})
		w.It("fails for different lengths", func(it *gotest.T) {
			m := newMock()
			gotest.ElementsMatch(m, []int{1, 2}, []int{1, 2, 3})
			gotest.True(it, m.failed)
		})
		w.It("fails for same elements different counts", func(it *gotest.T) {
			m := newMock()
			gotest.ElementsMatch(m, []int{1, 1, 2}, []int{1, 2, 2})
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestSubset(t *gotest.T) {
	t.When("subset is contained in list", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.Subset(m, []int{1, 2, 3, 4, 5}, []int{2, 4})
			gotest.False(it, m.failed)
		})
		w.It("passes for empty subset", func(it *gotest.T) {
			m := newMock()
			gotest.Subset(m, []int{1, 2, 3}, []int{})
			gotest.False(it, m.failed)
		})
	})

	t.When("subset has missing elements", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.Subset(m, []int{1, 2, 3}, []int{2, 99})
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestGreater(t *gotest.T) {
	t.When("a is greater than b", func(w *gotest.T) {
		w.It("passes for ints", func(it *gotest.T) {
			m := newMock()
			gotest.Greater(m, 5, 3)
			gotest.False(it, m.failed)
		})
		w.It("passes for floats", func(it *gotest.T) {
			m := newMock()
			gotest.Greater(m, 3.14, 2.71)
			gotest.False(it, m.failed)
		})
	})

	t.When("a is not greater", func(w *gotest.T) {
		w.It("fails when less", func(it *gotest.T) {
			m := newMock()
			gotest.Greater(m, 3, 5)
			gotest.True(it, m.failed)
		})
		w.It("fails when equal", func(it *gotest.T) {
			m := newMock()
			gotest.Greater(m, 4, 4)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestGreaterOrEqual(t *gotest.T) {
	t.When("a is greater than or equal to b", func(w *gotest.T) {
		w.It("passes when greater", func(it *gotest.T) {
			m := newMock()
			gotest.GreaterOrEqual(m, 5, 3)
			gotest.False(it, m.failed)
		})
		w.It("passes when equal", func(it *gotest.T) {
			m := newMock()
			gotest.GreaterOrEqual(m, 4, 4)
			gotest.False(it, m.failed)
		})
		w.It("passes for equal floats", func(it *gotest.T) {
			m := newMock()
			gotest.GreaterOrEqual(m, 3.14, 3.14)
			gotest.False(it, m.failed)
		})
	})

	t.When("a is less than b", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.GreaterOrEqual(m, 3, 5)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestLess(t *gotest.T) {
	t.When("a is less than b", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.Less(m, 3, 5)
			gotest.False(it, m.failed)
		})
	})

	t.When("a is not less", func(w *gotest.T) {
		w.It("fails when greater", func(it *gotest.T) {
			m := newMock()
			gotest.Less(m, 5, 3)
			gotest.True(it, m.failed)
		})
		w.It("fails when equal", func(it *gotest.T) {
			m := newMock()
			gotest.Less(m, 4, 4)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestLessOrEqual(t *gotest.T) {
	t.When("a is less than or equal to b", func(w *gotest.T) {
		w.It("passes when less", func(it *gotest.T) {
			m := newMock()
			gotest.LessOrEqual(m, 3, 5)
			gotest.False(it, m.failed)
		})
		w.It("passes when equal", func(it *gotest.T) {
			m := newMock()
			gotest.LessOrEqual(m, 4, 4)
			gotest.False(it, m.failed)
		})
	})

	t.When("a is greater than b", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.LessOrEqual(m, 5, 3)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestRegexp(t *gotest.T) {
	t.When("string matches pattern", func(w *gotest.T) {
		w.It("passes for string pattern", func(it *gotest.T) {
			m := newMock()
			gotest.Regexp(m, `^\d+$`, "12345")
			gotest.False(it, m.failed)
		})
		w.It("passes for compiled regexp", func(it *gotest.T) {
			m := newMock()
			re := regexp.MustCompile(`hello`)
			gotest.Regexp(m, re, "say hello world")
			gotest.False(it, m.failed)
		})
	})

	t.When("string does not match pattern", func(w *gotest.T) {
		w.It("fails for string pattern", func(it *gotest.T) {
			m := newMock()
			gotest.Regexp(m, `^\d+$`, "abc")
			gotest.True(it, m.failed)
		})
		w.It("fails for compiled regexp", func(it *gotest.T) {
			m := newMock()
			re := regexp.MustCompile(`^hello$`)
			gotest.Regexp(m, re, "say hello world")
			gotest.True(it, m.failed)
		})
	})

	t.When("pattern is invalid", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.Regexp(m, `[invalid`, "test")
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestInDelta(t *gotest.T) {
	t.When("values are within delta", func(w *gotest.T) {
		w.It("passes for floats", func(it *gotest.T) {
			m := newMock()
			gotest.InDelta(m, 3.14, 3.15, 0.02)
			gotest.False(it, m.failed)
		})
		w.It("passes for ints", func(it *gotest.T) {
			m := newMock()
			gotest.InDelta(m, 100, 101, 2.0)
			gotest.False(it, m.failed)
		})
	})

	t.When("values exceed delta", func(w *gotest.T) {
		w.It("fails for floats", func(it *gotest.T) {
			m := newMock()
			gotest.InDelta(m, 3.14, 3.50, 0.02)
			gotest.True(it, m.failed)
		})
		w.It("fails for ints", func(it *gotest.T) {
			m := newMock()
			gotest.InDelta(m, 100, 105, 2.0)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestJSONEq(t *gotest.T) {
	t.When("JSON structures are equal", func(w *gotest.T) {
		w.It("passes for different key order", func(it *gotest.T) {
			m := newMock()
			gotest.JSONEq(m, `{"a":1,"b":2}`, `{"b":2,"a":1}`)
			gotest.False(it, m.failed)
		})
		w.It("passes for []byte input", func(it *gotest.T) {
			m := newMock()
			gotest.JSONEq(m, []byte(`{"x":10}`), []byte(`{"x":10}`))
			gotest.False(it, m.failed)
		})
		w.It("passes for marshalable struct", func(it *gotest.T) {
			m := newMock()
			type S struct {
				A int `json:"a"`
			}
			gotest.JSONEq(m, S{A: 5}, `{"a":5}`)
			gotest.False(it, m.failed)
		})
		w.It("passes for io.Reader input", func(it *gotest.T) {
			m := newMock()
			reader := bytes.NewReader([]byte(`{"a":1,"b":2}`))
			gotest.JSONEq(m, reader, `{"b":2,"a":1}`)
			gotest.False(it, m.failed)
		})
		w.It("passes for json.RawMessage input", func(it *gotest.T) {
			m := newMock()
			raw := json.RawMessage(`{"x":10}`)
			gotest.JSONEq(m, raw, `{"x":10}`)
			gotest.False(it, m.failed)
		})
	})

	t.When("JSON structures differ", func(w *gotest.T) {
		w.It("fails for different values", func(it *gotest.T) {
			m := newMock()
			gotest.JSONEq(m, `{"a":1}`, `{"a":2}`)
			gotest.True(it, m.failed)
		})
		w.It("fails for different keys", func(it *gotest.T) {
			m := newMock()
			gotest.JSONEq(m, `{"a":1}`, `{"b":1}`)
			gotest.True(it, m.failed)
		})
	})

	t.When("input is invalid JSON", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.JSONEq(m, `{not json}`, `{"a":1}`)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestTimeWithin(t *gotest.T) {
	t.When("times are within tolerance", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			base := time.Now()
			gotest.TimeWithin(m, base, base.Add(50*time.Millisecond), 100*time.Millisecond)
			gotest.False(it, m.failed)
		})
	})

	t.When("times exceed tolerance", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			base := time.Now()
			gotest.TimeWithin(m, base, base.Add(200*time.Millisecond), 100*time.Millisecond)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestTimeIsNow(t *gotest.T) {
	t.When("time is recent", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.TimeIsNow(m, time.Now(), time.Second)
			gotest.False(it, m.failed)
		})
	})

	t.When("time is old", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.TimeIsNow(m, time.Now().Add(-time.Hour), time.Second)
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestPanics(t *gotest.T) {
	t.When("function panics", func(w *gotest.T) {
		w.It("passes and returns recovered value", func(it *gotest.T) {
			m := newMock()
			v := gotest.Panics(m, func() { panic("oh no") })
			gotest.False(it, m.failed)
			gotest.Equal(it, "oh no", v)
		})
	})

	t.When("function does not panic", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			gotest.Panics(m, func() {})
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestEventually(t *gotest.T) {
	t.When("condition becomes true before timeout", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			count := 0
			gotest.Eventually(m, 50*time.Millisecond, 1*time.Millisecond, func(poll *gotest.T) {
				count++
				gotest.GreaterOrEqual(poll, count, 3)
			})
			gotest.False(it, m.failed)
		})
	})

	t.When("condition never becomes true", func(w *gotest.T) {
		w.It("fails after timeout", func(it *gotest.T) {
			m := newMock()
			gotest.Eventually(m, 10*time.Millisecond, 1*time.Millisecond, func(poll *gotest.T) {
				gotest.True(poll, false)
			})
			gotest.True(it, m.failed)
		})
	})
}

func (s *AssertionsTestSuite) TestConsistently(t *gotest.T) {
	t.When("condition stays true for duration", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := newMock()
			gotest.Consistently(m, 20*time.Millisecond, 1*time.Millisecond, func(poll *gotest.T) {
				gotest.True(poll, true)
			})
			gotest.False(it, m.failed)
		})
	})

	t.When("condition becomes false", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := newMock()
			count := 0
			gotest.Consistently(m, 50*time.Millisecond, 1*time.Millisecond, func(poll *gotest.T) {
				count++
				gotest.Less(poll, count, 3)
			})
			gotest.True(it, m.failed)
		})
	})
}
