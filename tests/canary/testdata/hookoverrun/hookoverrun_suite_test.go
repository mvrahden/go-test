package hookoverrun

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type OverrunTestSuite struct {
	Slow *SlowFixture
}

func (s *OverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *OverrunTestSuite) BeforeEach(t *gotest.T) { mark("suite BeforeEach") }
func (s *OverrunTestSuite) TestSlow(t *gotest.T)   { mark("body") }

type ReleaseOverrunTestSuite struct {
	Release *SlowReleaseFixture
}

func (s *ReleaseOverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *ReleaseOverrunTestSuite) TestSlowRelease(t *gotest.T) {}

type WaitingTestSuite struct {
	Waiting *WaitingFixture
}

func (s *WaitingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *WaitingTestSuite) TestWaitedFor(t *gotest.T) {}

// WaitingBeforeEachTestSuite's BeforeEach waits on its context.
type WaitingBeforeEachTestSuite struct{}

func (s *WaitingBeforeEachTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *WaitingBeforeEachTestSuite) BeforeEach(t *gotest.T) { waitOn(t.Context(), "suite") }
func (s *WaitingBeforeEachTestSuite) TestAfterTheWait(t *gotest.T) {
	mark("body after the wait")
}
