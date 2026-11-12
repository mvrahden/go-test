package withdroppedcaseext_test

import (
	"withdroppedcaseext"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ProbeTestSuite struct{}

func (s *ProbeTestSuite) TestProbes(t *gotest.T) {
	p := &withdroppedcaseext.Probe{}
	p.TestProbe(t)
}

// The analysistest loader cannot load the source an alias would name, so a
// value spells the instance instead.
var _ withdroppedcaseext.ExtContractTestSuite[int]
