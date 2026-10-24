package lcaasync

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HelperFailsTestSuite: in every method a helper goroutine fails at once
// while the goroutine that owns done works on; AfterEach marks when the wait
// ended.
type HelperFailsTestSuite struct{ hang chan struct{} }

func (s *HelperFailsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 3 * time.Second
	return cfg
}

func (s *HelperFailsTestSuite) BeforeAll(t *gotest.T) { s.hang = make(chan struct{}) }
func (s *HelperFailsTestSuite) AfterAll(t *gotest.T)  { close(s.hang) }

func (s *HelperFailsTestSuite) AfterEach(t *gotest.T) { mark("AfterEach " + t.T().Name()) }

func helperFails(t *gotest.T) {
	gotest.Go(t, func() { gotest.Fail(t, "a helper goroutine fails") })
}

// TestOwnerCallsDoneAsync's owner calls done after the helper failed.
func (s *HelperFailsTestSuite) TestOwnerCallsDoneAsync(t *gotest.T, done func()) {
	helperFails(t)
	gotest.Go(t, func() {
		time.Sleep(700 * time.Millisecond)
		mark("owner called done")
		done()
	})
}

// TestOwnerFailsAsync's owner fails fatally after the helper failed.
func (s *HelperFailsTestSuite) TestOwnerFailsAsync(t *gotest.T, done func()) {
	helperFails(t)
	gotest.Go(t, func() {
		time.Sleep(700 * time.Millisecond)
		mark("owner failed")
		gotest.Fail(t, "the owner of done fails")
	})
}

// TestOwnerHangsAsync's owner never calls done; only AfterAll lets it go.
func (s *HelperFailsTestSuite) TestOwnerHangsAsync(t *gotest.T, done func()) {
	helperFails(t)
	go func() {
		<-s.hang
	}()
}

// HookOverrunTestSuite's method calls done at once; its AfterEach overruns
// the Timeout.
type HookOverrunTestSuite struct{}

func (s *HookOverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = time.Second
	return cfg
}

func (s *HookOverrunTestSuite) AfterEach(t *gotest.T) { time.Sleep(3 * time.Second) }

func (s *HookOverrunTestSuite) TestCallsDoneAsync(t *gotest.T, done func()) { done() }

// FatalTestSuite declares a generous Timeout, so a wait that ends early ends
// for a reason of its own.
type FatalTestSuite struct{}

func (s *FatalTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 20 * time.Second}
}

// TestFatalGoroutineAsync's goroutine fails fatally and never calls done.
func (s *FatalTestSuite) TestFatalGoroutineAsync(t *gotest.T, done func()) {
	gotest.Go(t, func() { gotest.Fail(t, "the goroutine that owns done fails") })
}

// TestCallsDoneAsync calls done from another goroutine and passes.
func (s *FatalTestSuite) TestCallsDoneAsync(t *gotest.T, done func()) {
	go func() {
		time.Sleep(10 * time.Millisecond)
		done()
	}()
}
