package xpkgconfig

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/canary/testdata/shapes/xpkgconfig/fix"
)

// BindsTestSuite binds a fixture from another package through a named field.
type BindsTestSuite struct {
	Configured *fix.ConfiguredFixture
}

func (s *BindsTestSuite) TestRuns(t *gotest.T) {}
