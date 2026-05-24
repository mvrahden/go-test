package gotest //nolint:stdlib-test

import (
	"testing"
)

func TestRecord_MultipleErrorf_LastWins(t *testing.T) {
	rec := Record(func(r *R) {
		r.Errorf("first")
		r.Errorf("second")
		r.Errorf("third")
	})
	if !rec.Failed() {
		t.Error("expected failure")
	}
	if rec.Message() != "third" {
		t.Errorf("message = %q, want %q (last-wins)", rec.Message(), "third")
	}
}

func TestRecord_FailNowStopsExecution(t *testing.T) {
	reached := false
	rec := Record(func(r *R) {
		r.FailNow()
		reached = true
	})
	if !rec.Failed() {
		t.Error("expected failure")
	}
	if reached {
		t.Error("code after FailNow should not execute")
	}
	if rec.Message() != "" {
		t.Errorf("message = %q, want empty (FailNow without Errorf)", rec.Message())
	}
}

func TestRecord_EmptyErrorf(t *testing.T) {
	rec := Record(func(r *R) {
		r.Errorf("")
	})
	if !rec.Failed() {
		t.Error("expected failure even with empty format")
	}
	if rec.Message() != "" {
		t.Errorf("message = %q, want empty string", rec.Message())
	}
}
