package fixaonce

import "github.com/mvrahden/go-test/pkg/gotest"

// BulkTestSuite runs with the concurrent suites and reads the fixture.
type BulkTestSuite struct {
	Once *OnceSharedFixture
}

func (s *BulkTestSuite) TestReadsOnce(t *gotest.T) {
	gotest.Equal(t, "up", s.Once.State)
	mark("bulk ran")
}

// TailTestSuite runs alone after the bulk and reads the same fixture.
type TailTestSuite struct {
	Once *OnceSharedFixture
}

func (s *TailTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *TailTestSuite) TestReadsOnce(t *gotest.T) {
	gotest.Equal(t, "up", s.Once.State)
	mark("tail ran")
}
