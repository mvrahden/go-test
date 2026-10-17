package exclusive_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Three suites, three processes. The two bulk suites hold their window open
// long enough to overlap each other; the Exclusive one must open only after
// both bulk windows closed. Every suite stamps its window into a file the
// canary reads back.

func stamp(event, suite string) {
	path := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "windows")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s %d\n", event, suite, time.Now().UnixNano())
}

func window(suite string, hold time.Duration) {
	stamp("start", suite)
	time.Sleep(hold)
	stamp("end", suite)
}

type BulkOneTestSuite struct{}

func (s *BulkOneTestSuite) TestWindow(t *gotest.T) { window("BulkOneTestSuite", 300*time.Millisecond) }

type BulkTwoTestSuite struct{}

func (s *BulkTwoTestSuite) TestWindow(t *gotest.T) { window("BulkTwoTestSuite", 300*time.Millisecond) }

type ExclusiveTestSuite struct{}

func (s *ExclusiveTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *ExclusiveTestSuite) TestWindow(t *gotest.T) {
	window("ExclusiveTestSuite", 50*time.Millisecond)
}
