// Package suitelifetime takes from BeforeAll's T what AfterAll reads back.
package suitelifetime

import (
	"os"
	"path/filepath"
	"sync"

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
