package gotest

import (
	"testing"

	"github.com/mvrahden/go-test/internal/harnessctor"
)

// The runtime builds these values for generated code; their fields are
// unexported here.
func init() {
	harnessctor.EachT = func(tb testing.TB) any { return &T{tb: tb} }
	harnessctor.BenchB = func(b *testing.B) any { return &B{b: b} }
}
