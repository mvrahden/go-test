// Package benchhang has a benchmark that hangs until the run's --timeout
// ends it, after one that finishes.
package benchhang

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type HangTestSuite struct{}

func (s *HangTestSuite) BenchmarkFast(b *gotest.B) {
	for b.Loop() {
	}
}

// BenchmarkHang ignores its context; LCB_WARM makes it return at once, for
// a run that only builds.
func (s *HangTestSuite) BenchmarkHang(b *gotest.B) {
	if os.Getenv("LCB_WARM") == "" {
		time.Sleep(time.Hour)
	}
	for b.Loop() {
	}
}
