package bench

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CacheFixture is a fixture-typed field (ends in "Fixture") for the
// bench-fixture-io testdata below.
type CacheFixture struct{}

type ParserBenchTestSuite struct{}

func (s *ParserBenchTestSuite) BenchmarkNoLoop(b *gotest.B) { // want `benchmark ParserBenchTestSuite.BenchmarkNoLoop never calls b.Loop\(\) — nothing is measured`
	_ = 1
}

func (s *ParserBenchTestSuite) BenchmarkWithLoop(b *gotest.B) {
	for b.Loop() {
		_ = 1
	}
}

// BenchmarkWithN accepts a stdlib *testing.B directly (also a valid
// benchmark signature) and iterates via the classic b.N idiom instead of
// b.Loop() — still "measures something", so bench-loop must not fire.
func (s *ParserBenchTestSuite) BenchmarkWithN(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = i
	}
}

func (s *ParserBenchTestSuite) BenchmarkSuppressed(b *gotest.B) { //nolint:bench-loop
	_ = 1
}

// CacheBenchTestSuite holds a fixture-typed field and a BeforeEach that
// rebuilds per-test/per-benchmark state from it — the shape the framework
// recommends (see examples/benchmarking). bench-fixture-io does not care
// about BeforeEach at all: gotest's generated wrapper fences the timer
// around it, so it can never pollute a measurement. What it does care about
// is whether a benchmark method's own body reads the fixture field *inside*
// its measured loop.
type CacheBenchTestSuite struct {
	fixture *CacheFixture
	cache   int
}

func (s *CacheBenchTestSuite) BeforeEach(t *gotest.T) {
	s.fixture = &CacheFixture{}
}

// BenchmarkQueryInsideLoop reads the fixture field from inside b.Loop() —
// that read is timed, so it fires.
func (s *CacheBenchTestSuite) BenchmarkQueryInsideLoop(b *gotest.B) { // want `benchmark CacheBenchTestSuite.BenchmarkQueryInsideLoop reads fixture-backed state s.fixture inside the measured loop — hoist the read above the loop, or you are timing whatever backs the fixture`
	for b.Loop() {
		_ = s.fixture
	}
}

// BenchmarkQueryHoisted reads the same fixture field, but above the loop —
// this is the pattern the framework recommends (see
// examples/benchmarking), and it used to false-positive under the old
// structural rule. It must stay clean.
func (s *CacheBenchTestSuite) BenchmarkQueryHoisted(b *gotest.B) {
	f := s.fixture
	for b.Loop() {
		_ = f
	}
}

// BenchmarkNestedRead buries the fixture read in a loop nested inside the
// measured loop. It must still fire: the rule walks the whole measured
// region, not just its direct children, and an implementation that only
// scanned the loop body's top level would silently miss this.
func (s *CacheBenchTestSuite) BenchmarkNestedRead(b *gotest.B) { // want `benchmark CacheBenchTestSuite.BenchmarkNestedRead reads fixture-backed state s.fixture inside the measured loop — hoist the read above the loop, or you are timing whatever backs the fixture`
	for b.Loop() {
		for i := 0; i < 4; i++ {
			_ = s.fixture
		}
	}
}

// BenchmarkClosureRead reads the fixture from a closure defined inside the
// measured loop — the same descent requirement as BenchmarkNestedRead.
func (s *CacheBenchTestSuite) BenchmarkClosureRead(b *gotest.B) { // want `benchmark CacheBenchTestSuite.BenchmarkClosureRead reads fixture-backed state s.fixture inside the measured loop — hoist the read above the loop, or you are timing whatever backs the fixture`
	for b.Loop() {
		func() { _ = s.fixture }()
	}
}

// BenchmarkNoFixtureRead has a fixture field and a BeforeEach (the old
// trigger shape) but never reads the fixture at all — clean.
func (s *CacheBenchTestSuite) BenchmarkNoFixtureRead(b *gotest.B) {
	for b.Loop() {
		s.cache++
	}
}

// BenchmarkNInsideLoop uses the classic b.N form with a fixture read
// inside the loop body — must fire the same as the b.Loop() case.
func (s *CacheBenchTestSuite) BenchmarkNInsideLoop(b *testing.B) { // want `benchmark CacheBenchTestSuite.BenchmarkNInsideLoop reads fixture-backed state s.fixture inside the measured loop — hoist the read above the loop, or you are timing whatever backs the fixture`
	for i := 0; i < b.N; i++ {
		_ = s.fixture
	}
}

// BenchmarkDocSuppressed reads the fixture inside the loop but is
// suppressed via a nolint directive that lives in its doc comment (last
// line of the block) rather than on the same line as the declaration — the
// doc-comment suppression path.
//
//nolint:bench-fixture-io
func (s *CacheBenchTestSuite) BenchmarkDocSuppressed(b *gotest.B) {
	for b.Loop() {
		_ = s.fixture
	}
}

// WaitBenchTestSuite exercises bench-wait: waiting primitives inside the
// measured loop time the wait, not the code.
type WaitBenchTestSuite struct{}

func (s *WaitBenchTestSuite) BenchmarkSleepInLoop(b *gotest.B) {
	for b.Loop() {
		time.Sleep(time.Millisecond) // want `benchmark WaitBenchTestSuite.BenchmarkSleepInLoop calls time.Sleep inside the measured loop — this times the wait, not the code; move it outside the loop`
	}
}

func (s *WaitBenchTestSuite) BenchmarkEventuallyInLoop(b *gotest.B) {
	for b.Loop() {
		gotest.Eventually(b, time.Second, time.Millisecond, func(poll *gotest.R) {}) // want `benchmark WaitBenchTestSuite.BenchmarkEventuallyInLoop calls gotest.Eventually inside the measured loop — this times the wait, not the code; move it outside the loop`
	}
}

func (s *WaitBenchTestSuite) BenchmarkConsistentlyInNLoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		gotest.Consistently(b, time.Second, time.Millisecond, func(poll *gotest.R) {}) // want `benchmark WaitBenchTestSuite.BenchmarkConsistentlyInNLoop calls gotest.Consistently inside the measured loop — this times the wait, not the code; move it outside the loop`
	}
}

// BenchmarkSleepAboveLoop settles before the measured region — clean.
func (s *WaitBenchTestSuite) BenchmarkSleepAboveLoop(b *gotest.B) {
	time.Sleep(time.Millisecond)
	for b.Loop() {
		_ = 1
	}
}

// BenchmarkSleepSuppressed opts out per line — a deliberate settle.
func (s *WaitBenchTestSuite) BenchmarkSleepSuppressed(b *gotest.B) {
	for b.Loop() {
		time.Sleep(time.Millisecond) //nolint:bench-wait
	}
}
