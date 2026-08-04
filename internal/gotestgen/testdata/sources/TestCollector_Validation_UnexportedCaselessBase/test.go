package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type baseTestSuite struct{}

func (s *baseTestSuite) helper() {}

type RealTestSuite struct {
	baseTestSuite
}

func (s *RealTestSuite) TestOne(t *gotest.T) {}
