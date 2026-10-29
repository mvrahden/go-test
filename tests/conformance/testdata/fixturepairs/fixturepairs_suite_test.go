package fixturepairs

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/fixturepairs/elsewhere"
)

// OverrunTestSuite: SlowFixture's BeforeEach overruns, ahead of LaterFixture's.
type OverrunTestSuite struct {
	Later *LaterFixture
}

func (s *OverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *OverrunTestSuite) TestBody(t *gotest.T) { mark("overrun.body") }

// AheadTestSuite: an AfterEach-only fixture before the one that fails.
type AheadTestSuite struct {
	Broken *BrokenFixture
}

func (s *AheadTestSuite) TestBody(t *gotest.T) { mark("ahead.body") }

// BehindTestSuite: an AfterEach-only fixture after the one that fails.
type BehindTestSuite struct {
	Behind *BehindFixture
}

func (s *BehindTestSuite) TestBody(t *gotest.T) { mark("behind.body") }

// RemoteTestSuite binds a fixture another package declares.
type RemoteTestSuite struct {
	Remote *elsewhere.RemoteFixture
}

func (s *RemoteTestSuite) TestBody(t *gotest.T) {}

// LeavingTestSuite binds another package's fixture whose AfterEach fails.
type LeavingTestSuite struct {
	Leaving *elsewhere.LeavingFixture
}

func (s *LeavingTestSuite) TestBody(t *gotest.T) {}
