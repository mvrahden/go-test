package togglegeneric

import "github.com/mvrahden/go-test/pkg/gotest"

func (s *ContractTestSuite[T]) TestCopy(t *gotest.T) {
	c := ContractTestSuite[T]{zero: s.zero}
	gotest.Equal(t, s.zero, c.zero)
}
