package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Storage interface{ Len() int }

type Value struct{ n int }

func (v Value) Len() int { return v.n }

// The hook is small enough to inline into the generated wrapper that calls it
// (go test -gcflags=-m: "inlining call to (*ContractTestSuite[...]).BeforeEach").
// The switch is the contract scaffold's shape, whose inlined body has hidden
// the hook's frame from the testing package.
type ContractTestSuite[T Storage] struct{ sut T }

func (s *ContractTestSuite[T]) BeforeEach(t *gotest.T) {
	switch any(s.sut).(type) {
	default:
		gotest.Fail(t, "no constructor for %T", s.sut)
	}
}

func (s *ContractTestSuite[T]) TestOne(t *gotest.T) {}

type ValueTestSuite = ContractTestSuite[Value]
type PointerTestSuite = ContractTestSuite[*Value]
