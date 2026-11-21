package d_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"rebuiltfirst.example/rebuiltfirst/mark"
)

// DTestSuite records that its package ran.
type DTestSuite struct{}

func (s *DTestSuite) TestRuns(t *gotest.T) { mark.Ran("d") }
