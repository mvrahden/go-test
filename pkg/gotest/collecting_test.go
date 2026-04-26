package gotest

import (
	"testing"
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
