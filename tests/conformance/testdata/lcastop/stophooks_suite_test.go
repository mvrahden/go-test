package lcastop

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LateBodyTestSuite's BeforeEach outlasts the stop; its body then ignores
// the context that has already ended.
type LateBodyTestSuite struct {
	Hold *HoldFixture
}

func (s *LateBodyTestSuite) BeforeEach(t *gotest.T) {
	mark("running")
	time.Sleep(time.Second)
}

func (s *LateBodyTestSuite) TestIgnores(t *gotest.T) {
	mark("body began")
	time.Sleep(60 * time.Second)
	mark("body returned")
}

// CleanupTestSuite's AfterEach waits on its context for a second, as cleanup
// that honours its context does.
type CleanupTestSuite struct {
	Hold *HoldFixture
}

func (s *CleanupTestSuite) AfterAll(t *gotest.T) { mark("suite.AfterAll") }

func (s *CleanupTestSuite) AfterEach(t *gotest.T) {
	select {
	case <-t.Context().Done():
		mark("AfterEach context ended")
	case <-time.After(time.Second):
		mark("AfterEach ran to its end")
	}
}

func (s *CleanupTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	<-t.Context().Done()
}

// StuckCleanupTestSuite's AfterEach ignores its context, so only the
// release bound ends a stopped run of it; the fixture takes a while to
// release. A SetupTimeout of 1s keeps that bound short.
type StuckCleanupTestSuite struct {
	Slow *SlowReleaseFixture
}

func (s *StuckCleanupTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: time.Second}
}

func (s *StuckCleanupTestSuite) AfterEach(t *gotest.T) { time.Sleep(time.Hour) }

func (s *StuckCleanupTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	<-t.Context().Done()
}
