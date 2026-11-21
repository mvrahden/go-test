package b_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"rebuiltfirst.example/rebuiltfirst/mark"
)

// BTestSuite records that its package ran.
type BTestSuite struct{}

func (s *BTestSuite) TestRuns(t *gotest.T) { mark.Ran("b") }
