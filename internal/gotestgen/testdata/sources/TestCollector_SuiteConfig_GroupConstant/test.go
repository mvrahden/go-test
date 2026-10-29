package testpkg

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"testpkg/groupnames"
)

const local = "local-1"

type LocalTestSuite struct{}

func (s *LocalTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.ExclusiveGroup = local
	return cfg
}
func (s *LocalTestSuite) TestOne(t *gotest.T) {}

type ForeignTestSuite struct{}

func (s *ForeignTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.ExclusiveGroup = groupnames.E2E
	return cfg
}
func (s *ForeignTestSuite) TestOne(t *gotest.T) {}
