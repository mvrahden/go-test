package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type ConfiguredTestSuite struct{}

func (s *ConfiguredTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 10_000_000_000, FailFast: true}
}
func (s *ConfiguredTestSuite) TestOne(t *gotest.T) {}
