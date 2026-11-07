package stopbudget

import (
	"os"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BudgetTestSuite holds until it is stopped. With its 1s SetupTimeout and
// its fixture's 1s Timeout it reports a teardown budget of 32s.
type BudgetTestSuite struct {
	Budget *BudgetFixture
}

func (s *BudgetTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: time.Second}
}

func (s *BudgetTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	<-t.Context().Done()
	mark("context ended")
}

// TestMain lingers after the tests for the duration STOPBUDGET_LINGER names,
// as a TestMain releasing what it set up would, and marks its end, which a
// process killed meanwhile never reaches.
func TestMain(m *testing.M) {
	code := m.Run()
	if d, err := time.ParseDuration(os.Getenv("STOPBUDGET_LINGER")); err == nil {
		time.Sleep(d)
		mark("lingered")
	}
	os.Exit(code)
}
