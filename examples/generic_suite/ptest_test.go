package genericsuite

import "github.com/mvrahden/go-test/pkg/gotest"

type GenericTestSuite[T any] struct {
	values []T
}

func (s *GenericTestSuite[T]) BeforeEach(t *gotest.T) {
	s.values = make([]T, 0)
}

func (s *GenericTestSuite[T]) TestAlpha(t *gotest.T) {
	gotest.Empty(t, s.values)
}

func (s *GenericTestSuite[T]) TestBeta(t *gotest.T) {
	var zero T
	s.values = append(s.values, zero)
	gotest.Len(t, s.values, 1)
}

type StringTestSuite = GenericTestSuite[string]

type IntTestSuite = GenericTestSuite[int]
