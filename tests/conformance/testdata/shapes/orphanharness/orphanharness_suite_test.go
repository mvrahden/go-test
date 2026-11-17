package orphanharness

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/shapes/orphanharness/harness"
)

// Each helper reaches the harness's interface one way only.
type argImpl struct{}

func (*argImpl) TestRoundTrip(t *gotest.T) { gotest.True(t, true) }

type assignImpl struct{}

func (*assignImpl) TestRoundTrip(t *gotest.T) { gotest.True(t, true) }

type returnImpl struct{}

func (*returnImpl) TestRoundTrip(t *gotest.T) { gotest.True(t, true) }

type literalImpl struct{}

func (*literalImpl) TestRoundTrip(t *gotest.T) { gotest.True(t, true) }

type convertImpl struct{}

func (*convertImpl) TestRoundTrip(t *gotest.T) { gotest.True(t, true) }

func newImpl() harness.Impl { return &returnImpl{} }

type HarnessTestSuite struct{}

func (s *HarnessTestSuite) TestArgument(t *gotest.T) { harness.Run(t, &argImpl{}) }

func (s *HarnessTestSuite) TestAssignment(t *gotest.T) {
	var i harness.Impl
	i = &assignImpl{}
	harness.Run(t, i)
}

func (s *HarnessTestSuite) TestReturn(t *gotest.T) { harness.Run(t, newImpl()) }

func (s *HarnessTestSuite) TestLiteral(t *gotest.T) {
	harness.RunAll(t, []harness.Impl{&literalImpl{}}...)
}

func (s *HarnessTestSuite) TestConversion(t *gotest.T) {
	i := harness.Impl(&convertImpl{})
	harness.Run(t, i)
}
