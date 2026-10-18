package passing_test

import "github.com/mvrahden/go-test/pkg/gotest"

// PassingTestSuite is green: the package every output-shape check runs.
type PassingTestSuite struct{}

func (s *PassingTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }
