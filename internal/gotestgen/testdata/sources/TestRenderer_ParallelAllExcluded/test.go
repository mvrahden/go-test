package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type QuietTestSuite struct{}

func (s *QuietTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *QuietTestSuite) X_TestOnly(t *gotest.T) {}
