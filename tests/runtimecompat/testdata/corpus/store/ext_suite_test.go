package store_test

import (
	"example.com/runtimecompat/store"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExtTestSuite struct{}

func (s *ExtTestSuite) TestNormalize(t *gotest.T) {
	gotest.Equal(t, "a", store.Normalize("A"))
}
