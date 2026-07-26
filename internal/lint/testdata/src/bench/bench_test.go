package bench

import (
	"testing"

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

// CacheBenchTestSuite defines BeforeEach and holds a fixture-typed field —
// combined, that's the bench-fixture-io shape.
type CacheBenchTestSuite struct {
	fixture *CacheFixture
}

func (s *CacheBenchTestSuite) BeforeEach(t *gotest.T) {
	s.fixture = &CacheFixture{}
}

func (s *CacheBenchTestSuite) BenchmarkQuery(b *gotest.B) { // want `benchmark CacheBenchTestSuite.BenchmarkQuery runs BeforeEach against fixture-backed state per method — measurements may include I/O`
	for b.Loop() {
		_ = s.fixture
	}
}
