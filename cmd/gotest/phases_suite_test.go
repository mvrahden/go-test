package main_test

import (
	"time"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PhasesTestSuite: the phase line a --debug run ends with.
type PhasesTestSuite struct{}

func (s *PhasesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *PhasesTestSuite) TestLine(t *gotest.T) {
	p := main.ExportPhases{Discover: 412*time.Millisecond + 300*time.Microsecond, Generate: 31 * time.Millisecond, Compile: 1204 * time.Millisecond, Run: 2310 * time.Millisecond, Report: 400 * time.Microsecond, Total: 2760 * time.Millisecond}
	gotest.Equal(t, "DEBUG: phases: discover=412ms generate=31ms compile=1.204s run=2.31s report=0s total=2.76s", p.String())
}
