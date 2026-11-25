package clean_test

import "github.com/mvrahden/go-test/pkg/gotest"

// CleanTestSuite gives lint nothing to report.
type CleanTestSuite struct{}

func (s *CleanTestSuite) TestAdds(t *gotest.T) {
	gotest.Equal(t, 2, 1+1)
}
