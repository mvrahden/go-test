package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

var enabled = true

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = enabled
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
