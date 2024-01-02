package gotestx

import "github.com/mvrahden/go-test/pkg/gotest"

type ComparableAsserter[T comparable] struct {
	t *gotest.T
}

func NewComparableAsserter[T comparable](t *gotest.T) ComparableAsserter[T] {
	return ComparableAsserter[T]{t: t}
}
