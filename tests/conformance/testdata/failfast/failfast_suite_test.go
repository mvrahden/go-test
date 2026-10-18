// Package failfast is a FailFast suite whose first method fails.
package failfast

import "github.com/mvrahden/go-test/pkg/gotest"

type FailFastTestSuite struct{}

func (s *FailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.FailFast = true
	return cfg
}

func (s *FailFastTestSuite) TestFirst(t *gotest.T)  { gotest.Fail(t, "the first method fails") }
func (s *FailFastTestSuite) TestSecond(t *gotest.T) { gotest.True(t, true) }
func (s *FailFastTestSuite) TestThird(t *gotest.T)  { gotest.True(t, true) }
