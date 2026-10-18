package nested

import "github.com/mvrahden/go-test/pkg/gotest"

// InnerTestSuite is a suite of its own.
type InnerTestSuite struct{}

func (s *InnerTestSuite) TestInner(t *gotest.T) {}

// OuterTestSuite holds a suite in a field.
type OuterTestSuite struct {
	Inner *InnerTestSuite
}

func (s *OuterTestSuite) TestOuter(t *gotest.T) {}
