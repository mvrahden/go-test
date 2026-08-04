package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PFTestSuite struct{}

func (s *PFTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, FailFast: true}
}
func (s *PFTestSuite) TestOne(t *gotest.T) {}
func (s *PFTestSuite) TestTwo(t *gotest.T) {}
