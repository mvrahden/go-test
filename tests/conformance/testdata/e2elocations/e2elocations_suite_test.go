package e2elocations

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LocationsTestSuite fails once in each place a failure is located from.
type LocationsTestSuite struct{}

func (s *LocationsTestSuite) TestGotestT(t *gotest.T) { gotest.True(t, false) }

func (s *LocationsTestSuite) TestTestingT(t *testing.T) { gotest.Equal(t, 1, 2) }

func (s *LocationsTestSuite) TestHelper(t *gotest.T) {
	expectEmpty(t, []int{1})
}

func expectEmpty(t *gotest.T, v []int) {
	gotest.Empty(t, v)
}
