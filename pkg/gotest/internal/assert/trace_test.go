// These tests verify the assertion primitives that gotest is built on. Using
// gotest suites here would be circular: a bug in the assertion logic would
// silently pass its own tests, making failures undetectable. stdlib tests with
// raw if/t.Error are the only way to independently verify correctness at this
// layer.
package assert_test //nolint:stdlib-test

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

func TestCallerTrace_DirectCall_ReturnsTrace(t *testing.T) {
	trace := assert.CallerTrace()
	if trace == "" {
		t.Fatalf("expected non-empty trace for direct call, got empty")
	}
	if !strings.Contains(trace, "called from:") {
		t.Fatalf("expected 'called from:' in trace, got: %q", trace)
	}
}

func helperThatCallsCallerTrace() string {
	return assert.CallerTrace()
}

func TestCallerTrace_SingleHelper_ReturnsTrace(t *testing.T) {
	trace := helperThatCallsCallerTrace()
	if trace == "" {
		t.Fatalf("expected non-empty trace for helper call, got empty")
	}
	if !strings.Contains(trace, "called from:") {
		t.Fatalf("expected 'called from:' in trace, got: %q", trace)
	}
	if !strings.Contains(trace, "trace_test.go:") {
		t.Fatalf("expected 'trace_test.go:' in trace, got: %q", trace)
	}
}

func simulateFailClosure() string {
	return assert.CallerTrace()
}

func simulateIsTrue() string {
	return simulateFailClosure()
}

func userHelper() string {
	return simulateIsTrue()
}

func TestCallerTrace_DeepHelperChain_ReturnsTrace(t *testing.T) {
	trace := userHelper()
	if trace == "" {
		t.Fatalf("expected non-empty trace for deep helper chain")
	}
	if !strings.Contains(trace, "called from:") {
		t.Fatalf("expected 'called from:' in trace, got: %q", trace)
	}
}

func TestCallerFrame_DirectCall_ReturnsFrame(t *testing.T) {
	frame := assert.CallerFrame()
	if frame == "" {
		t.Fatalf("expected non-empty frame, got empty")
	}
	if !strings.Contains(frame, "trace_test.go:") {
		t.Fatalf("expected 'trace_test.go:' in frame, got: %q", frame)
	}
	if strings.Contains(frame, "called from") {
		t.Fatalf("CallerFrame should not contain 'called from', got: %q", frame)
	}
}

func helperThatCallsCallerFrame() string {
	return assert.CallerFrame()
}

func TestCallerFrame_ThroughHelper_ReturnsOutermostUserFrame(t *testing.T) {
	frame := helperThatCallsCallerFrame()
	if frame == "" {
		t.Fatalf("expected non-empty frame, got empty")
	}
	if !strings.Contains(frame, "trace_test.go:") {
		t.Fatalf("expected 'trace_test.go:' in frame, got: %q", frame)
	}
}
