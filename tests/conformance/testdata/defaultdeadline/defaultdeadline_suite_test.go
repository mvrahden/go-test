// Package defaultdeadline has a suite that declares no Timeout, one of
// whose methods waits on its context until the default deadline ends it,
// and a suite whose method waits past the Timeout it declares.
package defaultdeadline

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type WaitingTestSuite struct{}

func (s *WaitingTestSuite) TestWaitsOnItsContext(t *gotest.T) { <-t.Context().Done() }
func (s *WaitingTestSuite) TestEndsAtOnce(t *gotest.T)        {}

type BudgetedTestSuite struct{}

func (s *BudgetedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Second}
}

func (s *BudgetedTestSuite) TestWaitsOnItsContext(t *gotest.T) { <-t.Context().Done() }
