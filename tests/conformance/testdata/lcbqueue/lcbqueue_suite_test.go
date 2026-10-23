// Package lcbqueue has a parallel suite with more methods than -parallel=1
// lets run at once, two of which hang until the run's --timeout ends them.
package lcbqueue

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type QueueTestSuite struct{}

func (s *QueueTestSuite) SuiteConfig() gotest.SuiteConfig { return gotest.SuiteConfig{Parallel: true} }

func (s *QueueTestSuite) TestHangA(t *gotest.T) { hang() }
func (s *QueueTestSuite) TestHangB(t *gotest.T) { hang() }
func (s *QueueTestSuite) TestQuick(t *gotest.T) {}

// hang ignores its context; LCB_WARM makes it return at once, for a run
// that only builds.
func hang() {
	if os.Getenv("LCB_WARM") == "" {
		time.Sleep(time.Hour)
	}
}
