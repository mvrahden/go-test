package gotest_test

import (
	"errors"
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ErrorAssertionsTestSuite covers the error assertions.
type ErrorAssertionsTestSuite struct{}

func (s *ErrorAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ErrorAssertionsTestSuite) TestNoError(t *gotest.T) {
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
			gotest.Contains(it, m.Message(), "some error")
		})
	})
}

func (s *ErrorAssertionsTestSuite) TestError(t *gotest.T) {
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
			gotest.Contains(it, m.Message(), "expected an error")
		})
	})
}

func (s *ErrorAssertionsTestSuite) TestErrorIs(t *gotest.T) {
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

	t.When("target is nil", func(w *gotest.T) {
		w.It("fails with type guard", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorIs(r, errSentinel, nil) }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "target is nil")
		})
		w.It("fails with type guard even when err is nil", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorIs(r, nil, nil) }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "target is nil")
		})
	})
}

func (s *ErrorAssertionsTestSuite) TestErrorAs(t *gotest.T) {
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
			m := gotest.Record(func(r *gotest.R) { _ = gotest.ErrorAs[*myError](r, errors.New("plain error")) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for nil error", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { _ = gotest.ErrorAs[*myError](r, nil) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *ErrorAssertionsTestSuite) TestErrorContains(t *gotest.T) {
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

	t.When("contains is empty", func(w *gotest.T) {
		w.It("fails with type guard", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorContains(r, errors.New("x"), "") }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "contains is empty")
		})
		w.It("fails with type guard even when err is nil", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorContains(r, nil, "") }) //nolint:assertion-simplify // testing guard behavior
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "contains is empty")
		})
	})
}
