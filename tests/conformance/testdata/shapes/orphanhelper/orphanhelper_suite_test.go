package orphanhelper

import "github.com/mvrahden/go-test/pkg/gotest"

// checks is a helper a suite calls on purpose.
type checks struct{}

func (c *checks) TestInvariant(t *gotest.T) { gotest.True(t, true) }

// viaInterface is a helper a suite calls through an interface.
type viaInterface struct{}

func (v *viaInterface) TestThroughInterface(t *gotest.T) { gotest.True(t, true) }

type caser interface{ TestThroughInterface(t *gotest.T) }

// Exported is a helper only the external test package calls.
type Exported struct{}

func (e *Exported) TestFromOutside(t *gotest.T) { gotest.True(t, true) }

type HelperTestSuite struct{ checks checks }

func (s *HelperTestSuite) TestCallsHelper(t *gotest.T) { s.checks.TestInvariant(t) }

func (s *HelperTestSuite) TestCallsThroughInterface(t *gotest.T) {
	var c caser = &viaInterface{}
	c.TestThroughInterface(t)
}
