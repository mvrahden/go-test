package benchfailing_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchFailingTestSuite declares the benchmark shapes that must not report a
// result: one fails, one halts at FailNow, one skips. The run is red and the
// report carries no sample for any of them.
type BenchFailingTestSuite struct{}

func (s *BenchFailingTestSuite) BenchmarkFailing(b *gotest.B) {
	b.Errorf("canary: this benchmark fails")
}

// BenchmarkHalting must stop at FailNow: the marker after it is never written.
func (s *BenchFailingTestSuite) BenchmarkHalting(b *gotest.B) {
	b.FailNow()
	_ = os.WriteFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "after-failnow"), []byte("ran"), 0o600)
}

func (s *BenchFailingTestSuite) BenchmarkSkipped(b *gotest.B) {
	b.Skipf("canary: skipped on purpose")
}
