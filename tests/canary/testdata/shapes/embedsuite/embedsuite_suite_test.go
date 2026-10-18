package embedsuite

import "github.com/mvrahden/go-test/pkg/gotest"

// BaseTestSuite would be a shared base.
type BaseTestSuite struct{}

func (s *BaseTestSuite) TestBase(t *gotest.T) {}

// ChildTestSuite embeds the base.
type ChildTestSuite struct {
	BaseTestSuite
}

func (s *ChildTestSuite) TestChild(t *gotest.T) {}
