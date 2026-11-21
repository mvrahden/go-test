package gotest

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/internal/harness"
	"github.com/mvrahden/go-test/internal/protocol"
)

// fuzzTarget is a generated wrapper's binding; pkg/gotestruntime names it.
type fuzzTarget = harness.FuzzTarget[*T]

// The runtime builds these values for generated code; their fields are
// unexported here.
func init() {
	harness.Registered = harness.Constructors[*T, *B, *F]{
		EachT:  func(tb testing.TB) *T { return &T{tb: tb} },
		BenchB: func(b *testing.B) *B { return &B{b: b} },
		FuzzF:  newF,
	}
}

func newF(f *testing.F, beforeEach, afterEach func(*T), target *fuzzTarget) *F {
	return &F{
		f: f, beforeEach: beforeEach, afterEach: afterEach, target: target,
		echoAll: os.Getenv(protocol.EnvFuzzEchoInput) == "1",
	}
}
