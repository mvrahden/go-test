package extcontract

import "github.com/mvrahden/go-test/pkg/gotest"

type IndexContractTestSuite[T any] struct{}

func (s *IndexContractTestSuite[T]) TestEmpty(t *gotest.T)  {}
func (s *IndexContractTestSuite[T]) TestInsert(t *gotest.T) {}
