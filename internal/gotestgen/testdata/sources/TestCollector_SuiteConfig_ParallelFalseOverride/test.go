package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.SuiteConfig{Parallel: true}
	cfg.Parallel = false
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
