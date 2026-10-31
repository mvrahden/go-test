package xpkgfixture

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/shapes/xpkgfixture/fix"
)

// BindsBadTestSuite binds a fixture from another package through a named field.
type BindsBadTestSuite struct {
	Bad *fix.BadFixture
}

func (s *BindsBadTestSuite) TestReady(t *gotest.T) { gotest.True(t, s.Bad.Ready) }
