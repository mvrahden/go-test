package benchhelper

import (
	"testing"

	"benchhelper/loops"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// runN loops on the stdlib B the benchmark's escape hands it.
func runN(tb *testing.B) {
	for i := 0; i < tb.N; i++ {
		_ = i
	}
}

// setup neither loops nor hands b on.
func setup(b *gotest.B) { b.ReportAllocs() }

// run is a function value: lint cannot read which body it holds.
var run = func(b *gotest.B) {
	for b.Loop() {
	}
}

type ReachTestSuite struct{}

func (s *ReachTestSuite) loop(b *gotest.B) {
	for b.Loop() {
	}
}

func (s *ReachTestSuite) BenchmarkEscape(b *gotest.B) { runN(b.B()) }

func (s *ReachTestSuite) BenchmarkMethod(b *gotest.B) { s.loop(b) }

func (s *ReachTestSuite) BenchmarkOtherPackage(b *gotest.B) { loops.Run(b, func() {}) }

func (s *ReachTestSuite) BenchmarkFuncValue(b *gotest.B) { run(b) }

func (s *ReachTestSuite) BenchmarkOnlyAsserts(b *gotest.B) { // want `benchmark ReachTestSuite\.BenchmarkOnlyAsserts never calls b\.Loop\(\) — nothing is measured`
	gotest.True(b, true)
}

func (s *ReachTestSuite) BenchmarkSetupOnly(b *gotest.B) { // want `benchmark ReachTestSuite\.BenchmarkSetupOnly never calls b\.Loop\(\) — nothing is measured`
	setup(b)
}
