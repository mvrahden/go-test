package fixataildep

import "github.com/mvrahden/go-test/pkg/gotest"

// BulkTestSuite binds nothing and runs before the tail.
type BulkTestSuite struct{}

func (s *BulkTestSuite) TestOne(t *gotest.T) { mark("bulk ran") }

// TailTestSuite runs alone, after the bulk, and reads the fixture whose
// dependency only it needs.
type TailTestSuite struct {
	App *AppSharedFixture
}

func (s *TailTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *TailTestSuite) TestReadsApp(t *gotest.T) {
	gotest.Equal(t, "up", s.App.State)
	mark("tail ran")
}
