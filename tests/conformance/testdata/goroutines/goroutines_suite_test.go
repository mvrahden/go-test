// Package goroutines starts goroutines with gotest.Go in the places that test
// where the wait on them ends.
package goroutines

import (
	"runtime"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ServerTestSuite starts a goroutine in BeforeAll that only AfterAll stops.
type ServerTestSuite struct{ stop chan struct{} }

func (s *ServerTestSuite) BeforeAll(t *gotest.T) {
	s.stop = make(chan struct{})
	gotest.Go(t, func() { <-s.stop })
}

func (s *ServerTestSuite) AfterAll(t *gotest.T) { close(s.stop) }

func (s *ServerTestSuite) TestServed(t *gotest.T) {}

// GoexitTestSuite's goroutine fails fatally before the method waits on it.
type GoexitTestSuite struct{}

func (s *GoexitTestSuite) TestGoexit(t *gotest.T) {
	wait := gotest.Go(t, func() { gotest.Fail(t, "from the goroutine") })
	wait()
	gotest.Fail(t, "the method went on after the wait")
}

// SkipTestSuite's goroutine skips the test.
type SkipTestSuite struct{}

func (s *SkipTestSuite) TestSkip(t *gotest.T) {
	wait := gotest.Go(t, func() { t.Skipf("skipped from the goroutine") })
	wait()
	gotest.Fail(t, "the method went on after the skip")
}

// RawGoexitTestSuite's goroutine leaves through runtime.Goexit with no
// verdict.
type RawGoexitTestSuite struct{}

func (s *RawGoexitTestSuite) TestRawGoexit(t *gotest.T) {
	gotest.Go(t, runtime.Goexit)()
}

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
