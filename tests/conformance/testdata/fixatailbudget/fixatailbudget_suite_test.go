package fixatailbudget

import "github.com/mvrahden/go-test/pkg/gotest"

// BulkTestSuite binds nothing and runs before the tail.
type BulkTestSuite struct{}

func (s *BulkTestSuite) TestOne(t *gotest.T) { mark("bulk ran") }

// TailTestSuite runs alone, after the bulk, and reads the late fixture.
type TailTestSuite struct {
	Late *LateSharedFixture
}

func (s *TailTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *TailTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "up", s.Late.State) }
