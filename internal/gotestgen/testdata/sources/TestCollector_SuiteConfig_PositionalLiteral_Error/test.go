package testpkg

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{30 * time.Second, 30 * time.Second, false, true}
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
