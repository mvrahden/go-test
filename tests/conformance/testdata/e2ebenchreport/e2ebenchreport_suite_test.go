package e2ebenchreport

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReportTestSuite has two benchmarks, so a selection can leave one out.
type ReportTestSuite struct{}

func (s *ReportTestSuite) BenchmarkKept(b *gotest.B) {
	for b.B().Loop() {
		_ = strings.Repeat("a", 4)
	}
}

func (s *ReportTestSuite) BenchmarkLeftOut(b *gotest.B) {
	for b.B().Loop() {
		_ = strings.Repeat("b", 4)
	}
}
