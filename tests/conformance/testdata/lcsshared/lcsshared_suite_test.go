package lcsshared

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// IgnoreTestSuite marks that it runs, then sleeps through the stop.
type IgnoreTestSuite struct {
	Probe *ProbeSharedFixture
}

func (s *IgnoreTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *IgnoreTestSuite) TestSleeps(t *gotest.T) {
	mark("running")
	time.Sleep(time.Minute)
}

// HeldTestSuite sleeps through the stop while it holds a package fixture
// beside the shared one.
type HeldTestSuite struct {
	Probe *ProbeSharedFixture
	Hold  *HoldFixture
}

func (s *HeldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *HeldTestSuite) TestSleeps(t *gotest.T) {
	mark("running")
	time.Sleep(time.Minute)
}

// StoppableTestSuite waits until its context ends, so a stop ends it at once.
type StoppableTestSuite struct {
	Probe *ProbeSharedFixture
}

func (s *StoppableTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *StoppableTestSuite) TestWaits(t *gotest.T) {
	mark("running")
	select {
	case <-t.Context().Done():
	case <-time.After(time.Minute):
	}
}

// FailFastTestSuite fails its first method; its second never runs.
type FailFastTestSuite struct {
	Probe *ProbeSharedFixture
}

func (s *FailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.FailFast = true
	return cfg
}

func (s *FailFastTestSuite) TestFirst(t *gotest.T) { gotest.Fail(t, "fails on purpose") }

func (s *FailFastTestSuite) TestSecond(t *gotest.T) { mark("second ran") }

// QuickTestSuite passes at once.
type QuickTestSuite struct {
	Probe *ProbeSharedFixture
}

func (s *QuickTestSuite) TestPasses(t *gotest.T) { gotest.Equal(t, "up", s.Probe.State) }
