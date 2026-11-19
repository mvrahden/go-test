package atrest

import (
	"fmt"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AtRestTestSuite only sleeps while it counts the process's allocations.
type AtRestTestSuite struct{}

// TestWaits marks the allocations of a short and a long window and what
// testing.AllocsPerRun reports for a sleep. A first window takes the
// one-off costs of the method's start; a poll shows as a long window that
// counts more than the short one.
func (s *AtRestTestSuite) TestWaits(t *gotest.T) {
	mallocsDuring(100 * time.Millisecond)
	mark(fmt.Sprintf("short=%d", mallocsDuring(200*time.Millisecond)))
	mark(fmt.Sprintf("long=%d", mallocsDuring(1200*time.Millisecond)))
	mark(fmt.Sprintf("allocs per run=%v", testing.AllocsPerRun(3, func() { time.Sleep(100 * time.Millisecond) })))
}
