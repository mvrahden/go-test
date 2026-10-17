package panics

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// RawTestSuite panics in a subtest it starts itself.
type RawTestSuite struct {
	Ledger *LedgerFixture
}

func (s *RawTestSuite) TestRaw(t *gotest.T) {
	t.T().Run("raw", func(*testing.T) { //nolint:suite-lifecycle // the scenario
		if Is("raw") {
			panic("raw subtest panic")
		}
	})
}

// ParallelTestSuite panics in a parallel subtest it starts itself, which
// runs after the body that started it has returned.
type ParallelTestSuite struct {
	Ledger *LedgerFixture
}

func (s *ParallelTestSuite) TestParallel(t *gotest.T) {
	t.T().Run("group", func(t *testing.T) { //nolint:suite-lifecycle // the scenario
		t.Run("par", func(t *testing.T) {
			t.Parallel()
			time.Sleep(10 * time.Millisecond)
			if Is("parallel") {
				panic("parallel subtest panic")
			}
		})
	})
}

// CleanupTestSuite panics in a cleanup it registers itself.
type CleanupTestSuite struct {
	Ledger *LedgerFixture
}

func (s *CleanupTestSuite) TestCleanup(t *gotest.T) {
	t.T().Cleanup(func() { //nolint:suite-lifecycle // the scenario
		if Is("cleanup") {
			panic("cleanup panic")
		}
	})
}

// FuzzSetupTestSuite panics in BeforeAll on the fuzz path.
type FuzzSetupTestSuite struct {
	Ledger *LedgerFixture
}

func (s *FuzzSetupTestSuite) BeforeAll(t *gotest.T) {
	if Is("fuzzbeforeall") {
		panic("fuzz BeforeAll panic")
	}
}

func (s *FuzzSetupTestSuite) AfterAll(t *gotest.T) {}

func (s *FuzzSetupTestSuite) FuzzInput(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}

// BenchTeardownTestSuite panics in AfterAll on the benchmark path.
type BenchTeardownTestSuite struct {
	Ledger *LedgerFixture
}

func (s *BenchTeardownTestSuite) BeforeAll(t *gotest.T) {}

func (s *BenchTeardownTestSuite) AfterAll(t *gotest.T) {
	if Is("benchafterall") {
		panic("bench AfterAll panic")
	}
}

func (s *BenchTeardownTestSuite) BenchmarkLoop(b *gotest.B) {
	for b.Loop() {
	}
}

// BenchSetupTestSuite panics in BeforeAll on the benchmark path.
type BenchSetupTestSuite struct {
	Ledger *LedgerFixture
}

func (s *BenchSetupTestSuite) BeforeAll(t *gotest.T) {
	if Is("benchbeforeall") {
		panic("bench BeforeAll panic")
	}
}

func (s *BenchSetupTestSuite) AfterAll(t *gotest.T) {}

func (s *BenchSetupTestSuite) BenchmarkLoop(b *gotest.B) {
	for b.Loop() {
	}
}

// BenchPanicTestSuite panics in a benchmark method of a fixture-bound suite.
type BenchPanicTestSuite struct {
	Ledger *LedgerFixture
}

func (s *BenchPanicTestSuite) BenchmarkPanics(b *gotest.B) {
	if Is("benchpanic") {
		panic("benchmark panic")
	}
	for b.Loop() {
	}
}

// UnboundBenchTestSuite panics in a benchmark method of a suite without
// fixtures.
type UnboundBenchTestSuite struct{}

func (s *UnboundBenchTestSuite) BenchmarkPanics(b *gotest.B) {
	if Is("unboundbench") {
		panic("unbound benchmark panic")
	}
	for b.Loop() {
	}
}
