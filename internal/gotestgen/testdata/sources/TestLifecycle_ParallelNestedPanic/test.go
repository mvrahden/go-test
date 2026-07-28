package testpkg

import (
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// NestedPanicTestSuite panics inside a nested When/It. The nested subtest owns
// its own goroutine, so its panic unwinds while the parallel test method above
// it is still parked inside t.Run — the exact shape that used to deadlock
// against a suite-scoped WaitGroup in t.Cleanup.
type NestedPanicTestSuite struct{}

func (s *NestedPanicTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *NestedPanicTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:afterall")
}

func (s *NestedPanicTestSuite) TestNestedPanics(t *gotest.T) {
	t.When("a nested behavior misbehaves", func(w *gotest.T) {
		w.It("panics", func(it *gotest.T) {
			panic("boom from a nested behavior")
		})
	})
}

func (s *NestedPanicTestSuite) TestSlowSibling(t *gotest.T) {
	time.Sleep(2 * time.Second)
	fmt.Println("MARK:done sibling")
}
