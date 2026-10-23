// Package failfastcleanup is a parallel FailFast suite whose methods fail
// only in a cleanup; run one at a time, the first must stop the other.
package failfastcleanup

import "github.com/mvrahden/go-test/pkg/gotest"

type CleanupFailFastTestSuite struct{}

func (s *CleanupFailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.FailFast = true
	cfg.Parallel = true
	return cfg
}

// failsInCleanup starts a goroutine that fails once its wait, a cleanup of
// the method, has begun: the cleanup registered after the wait releases it.
func failsInCleanup(t *gotest.T) {
	release := make(chan struct{})
	gotest.Go(t, func() {
		<-release
		gotest.Fail(t, "the goroutine failed")
	})
	t.T().Cleanup(func() { close(release) })
}

func (s *CleanupFailFastTestSuite) TestA(t *gotest.T) { failsInCleanup(t) }
func (s *CleanupFailFastTestSuite) TestB(t *gotest.T) { failsInCleanup(t) }
