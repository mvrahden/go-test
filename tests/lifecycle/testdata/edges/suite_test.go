// Package edges holds suites that fail in ways only a child process can show.
package edges

import (
	"runtime"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LingerTestSuite's goroutine ends a second after its method returned, five
// times the 200ms Timeout.
type LingerTestSuite struct{}

func (s *LingerTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 200 * time.Millisecond}
}

func (s *LingerTestSuite) TestLingers(t *gotest.T) {
	gotest.Go(t, func() { time.Sleep(time.Second) })
}

// SlowCleanupTestSuite's helper registers a cleanup that runs 500ms, past
// the 300ms Timeout, as a container's terminate would.
type SlowCleanupTestSuite struct{}

func (s *SlowCleanupTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 300 * time.Millisecond}
}

func (s *SlowCleanupTestSuite) TestSlowCleanup(t *gotest.T) {
	t.T().Cleanup(func() { time.Sleep(500 * time.Millisecond) }) //nolint:suite-lifecycle // stands for a third-party helper's cleanup
}

// GoexitAfterAllTestSuite's AfterAll leaves through runtime.Goexit, as a
// FailNow on some other testing.TB would make it.
type GoexitAfterAllTestSuite struct{}

func (s *GoexitAfterAllTestSuite) BeforeAll(t *gotest.T) {}

func (s *GoexitAfterAllTestSuite) AfterAll(t *gotest.T) { runtime.Goexit() }

func (s *GoexitAfterAllTestSuite) TestRuns(t *gotest.T) {}

// BoundTestSuite binds EdgeFixture, whose failures EDGES_FAIL picks.
type BoundTestSuite struct{ Edge *EdgeFixture }

func (s *BoundTestSuite) TestRuns(t *gotest.T) {}
