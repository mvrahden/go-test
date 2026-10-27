package fixaslots

import "github.com/mvrahden/go-test/pkg/gotest"

// AFirstTestSuite runs first in the exclusive tail and reads no fixture.
type AFirstTestSuite struct{}

func (s *AFirstTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *AFirstTestSuite) TestRuns(t *gotest.T) { mark("first ran") }

// BSecondTestSuite runs second and reads the late fixture.
type BSecondTestSuite struct {
	Late *LateSharedFixture
}

func (s *BSecondTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *BSecondTestSuite) TestReadsLate(t *gotest.T) {
	gotest.Equal(t, "up", s.Late.State)
	mark("second ran")
}
