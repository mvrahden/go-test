package gotest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// mockT captures test failures without actually failing the test.
type mockT struct {
	failed  bool
	message string
}

func (m *mockT) Helper() {}
func (m *mockT) Errorf(format string, args ...any) {
	m.failed = true
	m.message = fmt.Sprintf(format, args...)
}
func (m *mockT) FailNow() {
	m.failed = true
}

// newMock returns a fresh mockT.
func newMock() *mockT { return &mockT{} }

// ---- Equal ----

func TestEqual(t *testing.T) {
	t.Run("pass: same ints", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, 42, 42)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: different ints", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, 1, 2)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("pass: same strings", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, "hello", "hello")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: different strings", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, "hello", "world")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("pass: same slices", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, []int{1, 2, 3}, []int{1, 2, 3})
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: different slices", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, []int{1, 2}, []int{3, 4})
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- NotEqual ----

func TestNotEqual(t *testing.T) {
	t.Run("pass: different ints", func(t *testing.T) {
		m := newMock()
		gotest.NotEqual(m, 1, 2)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: same ints", func(t *testing.T) {
		m := newMock()
		gotest.NotEqual(m, 42, 42)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("pass: different strings", func(t *testing.T) {
		m := newMock()
		gotest.NotEqual(m, "hello", "world")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: same strings", func(t *testing.T) {
		m := newMock()
		gotest.NotEqual(m, "same", "same")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- True ----

func TestTrue(t *testing.T) {
	t.Run("pass: true", func(t *testing.T) {
		m := newMock()
		gotest.True(m, true)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: false", func(t *testing.T) {
		m := newMock()
		gotest.True(m, false)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- False ----

func TestFalse(t *testing.T) {
	t.Run("pass: false", func(t *testing.T) {
		m := newMock()
		gotest.False(m, false)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: true", func(t *testing.T) {
		m := newMock()
		gotest.False(m, true)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- Zero ----

func TestZero(t *testing.T) {
	t.Run("pass: int zero", func(t *testing.T) {
		m := newMock()
		gotest.Zero(m, 0)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: non-zero int", func(t *testing.T) {
		m := newMock()
		gotest.Zero(m, 42)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("pass: empty string zero", func(t *testing.T) {
		m := newMock()
		gotest.Zero(m, "")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: non-zero string", func(t *testing.T) {
		m := newMock()
		gotest.Zero(m, "hello")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- NotZero ----

func TestNotZero(t *testing.T) {
	t.Run("pass: non-zero int", func(t *testing.T) {
		m := newMock()
		gotest.NotZero(m, 42)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: zero int", func(t *testing.T) {
		m := newMock()
		gotest.NotZero(m, 0)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("pass: non-zero string", func(t *testing.T) {
		m := newMock()
		gotest.NotZero(m, "hello")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: zero string", func(t *testing.T) {
		m := newMock()
		gotest.NotZero(m, "")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- Empty ----

func TestEmpty(t *testing.T) {
	t.Run("pass: nil", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, nil)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("pass: empty slice", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, []int{})
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("pass: empty string", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, "")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("pass: empty map", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, map[string]int{})
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: non-empty slice", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, []int{1, 2, 3})
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("fail: non-empty string", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, "hello")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("fail: non-empty map", func(t *testing.T) {
		m := newMock()
		gotest.Empty(m, map[string]int{"a": 1})
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- NotEmpty ----

func TestNotEmpty(t *testing.T) {
	t.Run("pass: non-empty slice", func(t *testing.T) {
		m := newMock()
		gotest.NotEmpty(m, []int{1, 2, 3})
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("pass: non-empty string", func(t *testing.T) {
		m := newMock()
		gotest.NotEmpty(m, "hello")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: nil", func(t *testing.T) {
		m := newMock()
		gotest.NotEmpty(m, nil)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("fail: empty slice", func(t *testing.T) {
		m := newMock()
		gotest.NotEmpty(m, []int{})
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("fail: empty string", func(t *testing.T) {
		m := newMock()
		gotest.NotEmpty(m, "")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- NoError ----

func TestNoError(t *testing.T) {
	t.Run("pass: nil error", func(t *testing.T) {
		m := newMock()
		gotest.NoError(m, nil)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: non-nil error", func(t *testing.T) {
		m := newMock()
		gotest.NoError(m, errors.New("some error"))
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- Error ----

func TestError(t *testing.T) {
	t.Run("pass: non-nil error", func(t *testing.T) {
		m := newMock()
		gotest.Error(m, errors.New("some error"))
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: nil error", func(t *testing.T) {
		m := newMock()
		gotest.Error(m, nil)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- ErrorIs ----

var errSentinel = errors.New("sentinel error")

func TestErrorIs(t *testing.T) {
	t.Run("pass: direct match", func(t *testing.T) {
		m := newMock()
		gotest.ErrorIs(m, errSentinel, errSentinel)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("pass: wrapped error", func(t *testing.T) {
		m := newMock()
		wrapped := fmt.Errorf("wrapped: %w", errSentinel)
		gotest.ErrorIs(m, wrapped, errSentinel)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: different error", func(t *testing.T) {
		m := newMock()
		other := errors.New("other error")
		gotest.ErrorIs(m, other, errSentinel)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("fail: nil error", func(t *testing.T) {
		m := newMock()
		gotest.ErrorIs(m, nil, errSentinel)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- ErrorAs ----

type myError struct {
	Code int
}

func (e *myError) Error() string { return fmt.Sprintf("myError: code=%d", e.Code) }

func TestErrorAs(t *testing.T) {
	t.Run("pass: matching type", func(t *testing.T) {
		m := newMock()
		original := &myError{Code: 42}
		got := gotest.ErrorAs[*myError](m, original)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
		if got == nil || got.Code != 42 {
			t.Errorf("expected *myError with Code=42, got: %v", got)
		}
	})
	t.Run("pass: wrapped matching type", func(t *testing.T) {
		m := newMock()
		original := &myError{Code: 7}
		wrapped := fmt.Errorf("wrapped: %w", original)
		got := gotest.ErrorAs[*myError](m, wrapped)
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
		if got == nil || got.Code != 7 {
			t.Errorf("expected *myError with Code=7, got: %v", got)
		}
	})
	t.Run("fail: non-matching type", func(t *testing.T) {
		m := newMock()
		other := errors.New("plain error")
		gotest.ErrorAs[*myError](m, other)
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- ErrorContains ----

func TestErrorContains(t *testing.T) {
	t.Run("pass: substring present", func(t *testing.T) {
		m := newMock()
		gotest.ErrorContains(m, errors.New("file not found"), "not found")
		if m.failed {
			t.Errorf("expected no failure, got: %s", m.message)
		}
	})
	t.Run("fail: substring missing", func(t *testing.T) {
		m := newMock()
		gotest.ErrorContains(m, errors.New("file not found"), "connection refused")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
	t.Run("fail: nil error", func(t *testing.T) {
		m := newMock()
		gotest.ErrorContains(m, nil, "anything")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}

// ---- msgAndArgs propagation ----

func TestFailMessagePropagation(t *testing.T) {
	t.Run("message is included in failure output", func(t *testing.T) {
		m := newMock()
		gotest.Equal(m, 1, 2, "custom failure message")
		if !m.failed {
			t.Error("expected failure but got none")
		}
		if m.message == "" {
			t.Error("expected non-empty message")
		}
	})
	t.Run("format string message", func(t *testing.T) {
		m := newMock()
		gotest.True(m, false, "expected %s to be true", "value")
		if !m.failed {
			t.Error("expected failure but got none")
		}
	})
}
