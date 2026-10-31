package generic

import "github.com/mvrahden/go-test/pkg/gotest"

type ContractTestSuite[T comparable] struct{ zero T }

func (s *ContractTestSuite[T]) TestZero(t *gotest.T) {
	var v T
	gotest.Equal(t, s.zero, v)
}
