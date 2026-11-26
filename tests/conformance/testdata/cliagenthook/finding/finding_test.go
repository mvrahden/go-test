package finding_test

import "github.com/mvrahden/go-test/pkg/gotest"

// FindingTestSuite holds one lint finding: True over an == comparison.
type FindingTestSuite struct{}

func (s *FindingTestSuite) TestAdds(t *gotest.T) {
	gotest.True(t, 1+1 == 2)
}
