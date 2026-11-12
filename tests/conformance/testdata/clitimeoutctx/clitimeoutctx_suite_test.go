// Package clitimeoutctx has a method that waits for its context to end and
// then returns, so an expired --timeout stops a suite whose tests all pass.
package clitimeoutctx

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type WaitsTestSuite struct{}

// TestWaits returns at once in a warm-up run, so the build is cached.
func (s *WaitsTestSuite) TestWaits(t *gotest.T) {
	if os.Getenv("CLITIMEOUT_WARM") == "1" {
		return
	}
	<-t.Context().Done()
}
