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
