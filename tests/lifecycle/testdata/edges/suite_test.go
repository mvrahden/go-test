// Package edges holds suites that fail in ways only a child process can show.
package edges

import (
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
