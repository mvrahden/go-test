package fixatail

import "github.com/mvrahden/go-test/pkg/gotest"

// BulkTestSuite binds nothing and runs before the tail.
type BulkTestSuite struct{}

func (s *BulkTestSuite) TestOne(t *gotest.T) { mark("bulk ran") }

// TailTestSuite runs alone, after the bulk, and reads the fixture that hangs.
type TailTestSuite struct {
	Hang *HangSharedFixture
}

func (s *TailTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *TailTestSuite) TestNeverRuns(t *gotest.T) { mark("tail ran") }
