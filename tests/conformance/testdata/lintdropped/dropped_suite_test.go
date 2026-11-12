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
