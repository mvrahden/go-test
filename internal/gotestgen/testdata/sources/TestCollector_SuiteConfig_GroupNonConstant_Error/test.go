package testpkg

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.ExclusiveGroup = os.Getenv("GROUP")
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
