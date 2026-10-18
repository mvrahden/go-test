package genrename_test

import "github.com/mvrahden/go-test/pkg/gotest"

// ExtTestSuite is removed after the first generate.
type ExtTestSuite struct{}

func (s *ExtTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }
