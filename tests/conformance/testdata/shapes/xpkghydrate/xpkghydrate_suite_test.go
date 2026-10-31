package xpkghydrate

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/shapes/xpkghydrate/fix"
)

// BindsTestSuite binds a fixture from another package through a named field.
type BindsTestSuite struct {
	Hydrated *fix.HydratedFixture
}

func (s *BindsTestSuite) TestRuns(t *gotest.T) {}
