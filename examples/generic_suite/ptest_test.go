package genericsuite

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type GenericTestSuite[T any] struct {
	value T
}

func (s *GenericTestSuite[T]) BeforeEach(t *gotest.T) {
	Noop()
}

func (s *GenericTestSuite[T]) TestAlpha(t *gotest.T) {
	Noop()
}

func (s *GenericTestSuite[T]) TestBeta(t *gotest.T) {
	Noop()
}

type StringTestSuite = GenericTestSuite[string]

type IntTestSuite = GenericTestSuite[int]
