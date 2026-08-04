package testpkg

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.SuiteConfig{Parallel: true}
	cfg.Timeout = 2 * time.Minute
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
