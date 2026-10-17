package covered_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/covered"
)

// CoveredTestSuite covers every statement of its package, so any coverage
// gate above zero passes when the profile is read and fails when it is not.
type CoveredTestSuite struct{}

func (s *CoveredTestSuite) TestAdd(t *gotest.T) { gotest.Equal(t, 3, covered.Add(1, 2)) }
