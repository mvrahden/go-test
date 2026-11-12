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

// GoexitAfterAllTestSuite's AfterAll leaves through runtime.Goexit, as a
// FailNow on some other testing.TB would make it.
type GoexitAfterAllTestSuite struct{}

func (s *GoexitAfterAllTestSuite) BeforeAll(t *gotest.T) {}

func (s *GoexitAfterAllTestSuite) AfterAll(t *gotest.T) { runtime.Goexit() }

func (s *GoexitAfterAllTestSuite) TestRuns(t *gotest.T) {}

// BoundTestSuite binds EdgeFixture, whose failures EDGES_FAIL picks.
type BoundTestSuite struct{ Edge *EdgeFixture }

func (s *BoundTestSuite) TestRuns(t *gotest.T) {}
