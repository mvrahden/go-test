package good_test

import "github.com/mvrahden/go-test/pkg/gotest"

type GoodTestSuite struct{}

func (s *GoodTestSuite) TestHolds(t *gotest.T) { gotest.Equal(t, 1, 1) }
