package withdroppedcase_test

import (
	"withdroppedcase"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ProbeTestSuite struct{}

func (s *ProbeTestSuite) TestProbes(t *gotest.T) {
	p := &withdroppedcase.Probe{}
	p.TestProbe(t)
}

// The analysistest loader cannot load the source an alias would name, so a
// value spells the instance instead.
var _ withdroppedcase.ExtContractTestSuite[int]
