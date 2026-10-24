// Package asyncfatal holds async methods whose done never comes the ordinary way.
package asyncfatal

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncTestSuite declares a generous Timeout, so a wait that ends early ends
// for a reason of its own.
type AsyncTestSuite struct{}

func (s *AsyncTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 20 * time.Second
	return cfg
}

// TestFatalGoroutineAsync's goroutine fails fatally and never calls done.
func (s *AsyncTestSuite) TestFatalGoroutineAsync(t *gotest.T, done func()) {
	gotest.Go(t, func() { gotest.Fail(t, "the goroutine that owns done fails") })
}

// TestCallsDoneAsync calls done from another goroutine and passes.
func (s *AsyncTestSuite) TestCallsDoneAsync(t *gotest.T, done func()) {
	go func() {
		time.Sleep(10 * time.Millisecond)
		done()
	}()
}
