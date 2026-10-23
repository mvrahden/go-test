// Package failfastsoft is a FailFast suite whose BeforeAll records a failure
// without stopping, in a behavior block; no method of it fails.
package failfastsoft

import "github.com/mvrahden/go-test/pkg/gotest"

type SoftFailFastTestSuite struct{}

func (s *SoftFailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.FailFast = true
	return cfg
}

func (s *SoftFailFastTestSuite) BeforeAll(t *gotest.T) {
	t.It("checks the environment", func(it *gotest.T) {
		gotest.Fail(it, "the check fails, and BeforeAll goes on")
	})
}

func (s *SoftFailFastTestSuite) AfterAll(t *gotest.T) {}

func (s *SoftFailFastTestSuite) TestFirst(t *gotest.T)  { gotest.True(t, true) }
func (s *SoftFailFastTestSuite) TestSecond(t *gotest.T) { gotest.True(t, true) }
