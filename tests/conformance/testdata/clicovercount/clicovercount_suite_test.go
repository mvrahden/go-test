package clicovercount_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/clicovercount"
)

type FirstTestSuite struct{}

func (s *FirstTestSuite) TestHit(t *gotest.T) { gotest.Equal(t, 1, clicovercount.Hit()) }

type SecondTestSuite struct{}

func (s *SecondTestSuite) TestHit(t *gotest.T) { gotest.Equal(t, 1, clicovercount.Hit()) }
