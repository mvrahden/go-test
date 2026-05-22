package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

var cfg = gotest.SuiteConfig{Parallel: true}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
