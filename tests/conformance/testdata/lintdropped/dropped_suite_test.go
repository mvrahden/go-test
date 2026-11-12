package lintdropped

import "github.com/mvrahden/go-test/pkg/gotest"

// UserSuite lacks the suffix: nothing runs TestCreate.
type UserSuite struct{}

func (s *UserSuite) TestCreate(t *gotest.T) { gotest.Fail(t, "never runs") }

// Harness is a helper: the suite below calls its TestConnection.
type Harness struct{}

func (h *Harness) TestConnection(t *gotest.T) { gotest.True(t, true) }

type PayTestSuite struct{ h Harness }

func (s *PayTestSuite) TestPays(t *gotest.T) { s.h.TestConnection(t) }

func (s *PayTestSuite) Test(t *gotest.T) { gotest.Fail(t, "never runs") }

func (s *PayTestSuite) Benchmark(b *gotest.B) {
	for b.Loop() {
	}
}

func (s *PayTestSuite) Fuzz(f *gotest.F) { f.Fuzz(func(t *gotest.T, n int) {}) }

// OrphanContractTestSuite has no alias: nothing runs TestHolds.
type OrphanContractTestSuite[T any] struct{}

func (s *OrphanContractTestSuite[T]) TestHolds(t *gotest.T) { gotest.Fail(t, "never runs") }

// StoreContractTestSuite runs as IntStoreTestSuite.
type StoreContractTestSuite[T any] struct{}

func (s *StoreContractTestSuite[T]) TestHolds(t *gotest.T) {}

type IntStoreTestSuite = StoreContractTestSuite[int]

// Checker's cases are reached through an interface and as a method value.
type Checker struct{}

func (c *Checker) TestReady(t *gotest.T) {}

func (c *Checker) TestValue(t *gotest.T) {}

type readier interface{ TestReady(t *gotest.T) }

type CheckTestSuite struct{ r readier }

func (s *CheckTestSuite) TestChecks(t *gotest.T) {
	if s.r != nil {
		s.r.TestReady(t)
	}
	f := (&Checker{}).TestValue
	_ = f
}

// Pair has two cases nothing runs, one marked off with X_.
type Pair struct{}

func (p *Pair) TestOne(t *gotest.T) {}

func (p *Pair) X_TestTwo(t *gotest.T) {}

// Box is a generic type that is no suite.
type Box[T any] struct{}

func (b *Box[T]) TestBoxed(t *gotest.T) {}

// Shared is declared in a non-test file, which also declares a case on it.
func (s *Shared) TestAdded(t *gotest.T) {}
