package testpkg

import (
	"testpkg/extbadfixture"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BindsBadTestSuite struct {
	Bad *extbadfixture.BadFixture
}

func (s *BindsBadTestSuite) TestOne(t *gotest.T) {}
