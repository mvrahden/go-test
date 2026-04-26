package assert_test

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

func TestCallerTrace_DirectCall_ReturnsEmpty(t *testing.T) {
	trace := assert.CallerTrace()
	if trace != "" {
		t.Fatalf("expected empty trace for direct call, got: %q", trace)
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
