// Package suitelifetime takes from BeforeAll's T what AfterAll reads back.
package suitelifetime

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// ResourcesTestSuite takes a temp dir and an environment variable in BeforeAll.
type ResourcesTestSuite struct{ dir string }

func (s *ResourcesTestSuite) BeforeAll(t *gotest.T) {
	s.dir = t.TempDir()
	t.Setenv("SUITELIFETIME_ENV", "set")
}

func (s *ResourcesTestSuite) AfterAll(t *gotest.T) {
	if _, err := os.Stat(s.dir); err == nil {
		mark("afterall: dir kept")
	}
	if os.Getenv("SUITELIFETIME_ENV") == "set" {
		mark("afterall: env kept")
	}
}

func (s *ResourcesTestSuite) TestRuns(t *gotest.T) {}

// LongTestSuite's method runs four times its 300ms SetupTimeout.
type LongTestSuite struct{ ctx context.Context }

func (s *LongTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 300 * time.Millisecond, Timeout: 30 * time.Second}
}

func (s *LongTestSuite) BeforeAll(t *gotest.T) { s.ctx = t.Context() }

func (s *LongTestSuite) AfterAll(t *gotest.T) {
	if s.ctx.Err() != nil {
		mark("afterall: context ended")
	}
}

func (s *LongTestSuite) TestOutlastsSetup(t *gotest.T) {
	time.Sleep(1200 * time.Millisecond)
	gotest.NoError(t, s.ctx.Err(), "the context BeforeAll got ended mid-suite")
	mark("method: context live")
}

// WaitingTestSuite's BeforeAll waits on its context past its 300ms SetupTimeout.
type WaitingTestSuite struct{}

func (s *WaitingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 300 * time.Millisecond}
}

func (s *WaitingTestSuite) BeforeAll(t *gotest.T) {
	select {
	case <-t.Context().Done():
		mark("beforeall: context ended")
	case <-time.After(5 * time.Second):
	}
}

func (s *WaitingTestSuite) AfterAll(t *gotest.T) {}

func (s *WaitingTestSuite) TestRuns(t *gotest.T) {}
