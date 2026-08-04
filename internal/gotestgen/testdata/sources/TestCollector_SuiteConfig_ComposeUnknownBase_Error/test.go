package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) custom() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := s.custom()
	cfg.FailFast = true
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
