// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestruntime_test //nolint:fail-guard

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// LifetimesTestSuite: how long what a hook takes from its T lasts.
// Sequential: BeforeAll sets an environment variable, which testing refuses
// below a parallel test.
type LifetimesTestSuite struct{}

func (s *LifetimesTestSuite) TestBeforeAllResourcesLastThroughAfterAll(t *gotest.T) {
	const key = "GOTEST_LIFETIMES_PROBE"
	var dir, env string
	var statErr error
	t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // OpenSuite takes the raw testing.TB
		gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(bt *gotest.T) { dir = bt.TempDir(); bt.Setenv(key, "set") },
			AfterAll:  func(*gotest.T) { _, statErr = os.Stat(dir); env = os.Getenv(key) },
		})
	})
	if statErr != nil {
		t.Errorf("AfterAll found BeforeAll's temp dir gone: %v", statErr)
	}
	if env != "set" {
		t.Errorf("AfterAll found BeforeAll's environment variable undone: %q", env)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("BeforeAll's temp dir outlived the suite: %v", err)
	}
	if got := os.Getenv(key); got != "" {
		t.Errorf("BeforeAll's environment variable outlived the suite: %q", got)
	}
}
