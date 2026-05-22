package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *MyTestSuite) BeforeEach(t *gotest.T) {}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
