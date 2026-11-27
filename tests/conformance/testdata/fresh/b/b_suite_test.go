package b_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"fresh/b"
)

type BTestSuite struct{}

func (s *BTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, 1, b.One()) }
