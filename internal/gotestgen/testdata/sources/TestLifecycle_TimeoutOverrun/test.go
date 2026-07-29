package testpkg

import (
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TimeoutOverrunTestSuite blows its configured per-test budget while ignoring
// the context it was given, which is what real code that blocks on I/O does.
// Go cannot preempt it, so the overrun has to surface after the fact.
type TimeoutOverrunTestSuite struct{}

func (s *TimeoutOverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 200 * time.Millisecond}
}

func (s *TimeoutOverrunTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:suite afterall")
}

func (s *TimeoutOverrunTestSuite) TestOverrunsItsBudget(t *gotest.T) {
	time.Sleep(1500 * time.Millisecond)
	fmt.Println("MARK:overrunning test returned")
}

func (s *TimeoutOverrunTestSuite) TestStaysWithinBudget(t *gotest.T) {
	fmt.Println("MARK:fast test returned")
}
