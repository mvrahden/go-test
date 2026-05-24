package gotest //nolint:stdlib-test

import (
	"testing"
)

func TestRecord_Pass(t *testing.T) {
	rec := Record(func(r *R) {
		// no assertions fail
	})
	if rec.Failed() {
		t.Errorf("expected pass, got failed with message: %s", rec.Message())
	}
}

func TestRecord_Errorf(t *testing.T) {
	rec := Record(func(r *R) {
		r.Errorf("expected %d, got %d", 1, 2)
	})
	if !rec.Failed() {
		t.Error("expected failure")
	}
	if rec.Message() != "expected 1, got 2" {
		t.Errorf("message = %q, want %q", rec.Message(), "expected 1, got 2")
	}
}

func TestRecord_FailNow(t *testing.T) {
	rec := Record(func(r *R) {
		r.FailNow()
		t.Error("should not reach here after FailNow")
	})
	if !rec.Failed() {
		t.Error("expected failure from FailNow")
	}
}

func TestRecord_ErrorfThenFailNow(t *testing.T) {
	rec := Record(func(r *R) {
		r.Errorf("first error")
		r.FailNow()
	})
	if !rec.Failed() {
		t.Error("expected failure")
	}
	if rec.Message() != "first error" {
		t.Errorf("message = %q, want %q", rec.Message(), "first error")
	}
}

func TestRecord_MultipleErrorf_LastWins(t *testing.T) {
	rec := Record(func(r *R) {
		r.Errorf("first")
		r.Errorf("second")
		r.Errorf("third")
	})
	True(t, rec.Failed())
	Equal(t, "third", rec.Message())
}

func TestRecord_FailNowStopsExecution(t *testing.T) {
	reached := false
	rec := Record(func(r *R) {
		r.FailNow()
		reached = true
	})
	True(t, rec.Failed())
	False(t, reached)
	Equal(t, "", rec.Message())
}

func TestRecord_EmptyErrorf(t *testing.T) {
	rec := Record(func(r *R) {
		r.Errorf("")
	})
	True(t, rec.Failed())
	Equal(t, "", rec.Message())
}
