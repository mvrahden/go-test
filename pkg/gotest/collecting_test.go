package gotest //nolint:stdlib-test

import (
	"testing"
	"time"
)

func TestRunCollectingPoll_Pass(t *testing.T) {
	failed, msg := runCollectingPoll(func(poll *T) {
		// no assertions fail
	})
	if failed {
		t.Errorf("expected pass, got failed with message: %s", msg)
	}
}

func TestRunCollectingPoll_Errorf(t *testing.T) {
	failed, msg := runCollectingPoll(func(poll *T) {
		poll.Errorf("expected %d, got %d", 1, 2)
	})
	if !failed {
		t.Error("expected failure")
	}
	if msg != "expected 1, got 2" {
		t.Errorf("message = %q, want %q", msg, "expected 1, got 2")
	}
}

func TestRunCollectingPoll_FailNow(t *testing.T) {
	failed, _ := runCollectingPoll(func(poll *T) {
		poll.FailNow()
		t.Error("should not reach here after FailNow")
	})
	if !failed {
		t.Error("expected failure from FailNow")
	}
}

func TestRunCollectingPoll_ErrorfThenFailNow(t *testing.T) {
	failed, msg := runCollectingPoll(func(poll *T) {
		poll.Errorf("first error")
		poll.FailNow()
	})
	if !failed {
		t.Error("expected failure")
	}
	if msg != "first error" {
		t.Errorf("message = %q, want %q", msg, "first error")
	}
}

func TestNewTWithDeadline_SetsContextDeadline(t *testing.T) {
	tt := NewTWithDeadline(t, 5*time.Second)
	deadline, ok := tt.Context().Deadline()
	if !ok {
		t.Fatal("expected context to have a deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 5*time.Second {
		t.Fatalf("expected deadline within 5s, got %v remaining", remaining)
	}
}

func TestNewTWithDeadline_ContextCancelledOnTimeout(t *testing.T) {
	tt := NewTWithDeadline(t, 10*time.Millisecond)
	<-tt.Context().Done()
	if tt.Context().Err() == nil {
		t.Fatal("expected context error after timeout")
	}
}

func TestNewTWithDeadline_PreservesTestingT(t *testing.T) {
	tt := NewTWithDeadline(t, 1*time.Second)
	if tt.T() != t {
		t.Fatal("expected T() to return the original *testing.T")
	}
}

func TestT_Context_UsesCustomCtxWhenSet(t *testing.T) {
	tt := NewTWithDeadline(t, 1*time.Second)
	if tt.ctx == nil {
		t.Fatal("expected custom ctx to be set")
	}
	ctx := tt.Context()
	if ctx != tt.ctx {
		t.Fatal("expected Context() to return the custom ctx")
	}
}

func TestT_Context_FallsBackToTestingContext(t *testing.T) {
	tt := NewT(t)
	if tt.ctx != nil {
		t.Fatal("expected ctx to be nil for NewT")
	}
	ctx := tt.Context()
	if ctx != t.Context() {
		t.Fatal("expected Context() to fall back to testing.T.Context()")
	}
}
