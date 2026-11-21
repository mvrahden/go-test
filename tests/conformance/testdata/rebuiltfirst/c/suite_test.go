package c_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"rebuiltfirst.example/rebuiltfirst/mark"
)

// CTestSuite records that its package ran.
type CTestSuite struct{}

func (s *CTestSuite) TestRuns(t *gotest.T) { mark.Ran("c") }
