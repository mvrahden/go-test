package aliasalias

import "github.com/mvrahden/go-test/pkg/gotest"

// GenericTestSuite is instantiated through its aliases.
type GenericTestSuite[T any] struct{}

func (s *GenericTestSuite[T]) TestZero(t *gotest.T) {
	var v T
	gotest.Equal(t, v, *new(T))
}

type IntTestSuite = GenericTestSuite[int]

// IntAgainTestSuite is an alias of an alias.
type IntAgainTestSuite = IntTestSuite
