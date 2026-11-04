package e2ebenchreport

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReportTestSuite has two benchmarks, so a selection can leave one out. Each
// sleeps: a per-op time of 20ms barely moves under load, so a wide gate holds.
type ReportTestSuite struct{}

func (s *ReportTestSuite) BenchmarkKept(b *gotest.B) {
	for b.Loop() {
		time.Sleep(20 * time.Millisecond) //nolint:bench-wait // the wait is the stable per-op time the gate clause needs
	}
}

func (s *ReportTestSuite) BenchmarkLeftOut(b *gotest.B) {
	for b.Loop() {
		time.Sleep(20 * time.Millisecond) //nolint:bench-wait // the wait is the stable per-op time the gate clause needs
	}
}
