package a_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"fresh/a"
)

type ATestSuite struct{}

func (s *ATestSuite) TestTwo(t *gotest.T) { gotest.Equal(t, 2, a.Two()) }

func (s *ATestSuite) TestAgain(t *gotest.T) { gotest.Equal(t, 2, a.Two()) }
