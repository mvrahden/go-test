package tracelatetwice

import "github.com/mvrahden/go-test/pkg/gotest"

type AFirstTestSuite struct {
	Late *LateSharedFixture
}

func (s *AFirstTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *AFirstTestSuite) TestReadsLate(t *gotest.T) { gotest.Equal(t, "up", s.Late.State) }

type BSecondTestSuite struct {
	Late *LateSharedFixture
}

func (s *BSecondTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *BSecondTestSuite) TestReadsLate(t *gotest.T) { gotest.Equal(t, "up", s.Late.State) }
