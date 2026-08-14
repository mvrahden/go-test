package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
