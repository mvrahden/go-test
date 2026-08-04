package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

// DefaultSuiteConfig shadows the gotest preset name but returns Parallel: true —
// accepting it by name alone would silently generate a sequential suite.
func (s *MyTestSuite) DefaultSuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return s.DefaultSuiteConfig()
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
