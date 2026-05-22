package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteConfig(x int) gotest.SuiteConfig {
	return gotest.DefaultSuiteConfig()
}
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
