package a_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"rebuiltfirst.example/rebuiltfirst/mark"
)

// ATestSuite records that its package ran.
type ATestSuite struct{}

func (s *ATestSuite) TestRuns(t *gotest.T) { mark.Ran("a") }
