// Package lintextent holds one finding on a call and one on a function; see
// the lint conformance suite.
package lintextent_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExtentTestSuite struct{}

func (s *ExtentTestSuite) TestEqual(t *gotest.T) {
	a, b := 1, 1
	gotest.True(t, a == b)
}

func TestPlain(t *testing.T) {
	_ = t
}
