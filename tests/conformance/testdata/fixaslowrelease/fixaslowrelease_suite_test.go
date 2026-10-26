package fixaslowrelease

import "github.com/mvrahden/go-test/pkg/gotest"

// BulkTestSuite is the slow fixture's only reader.
type BulkTestSuite struct {
	Slow *SlowSharedFixture
}

func (s *BulkTestSuite) TestReadsSlow(t *gotest.T) { gotest.Equal(t, "up", s.Slow.State) }

// TailTestSuite runs alone after the bulk and reads a fixture of its own.
type TailTestSuite struct {
	Tail *TailSharedFixture
}

func (s *TailTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *TailTestSuite) TestReadsTail(t *gotest.T) {
	gotest.Equal(t, "up", s.Tail.State)
	mark("tail ran")
}
