package benching_test

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// JoinTestSuite has one benchmark, enough for a baseline to be saved,
// compared and gated.
type JoinTestSuite struct{}

func (s *JoinTestSuite) BenchmarkJoin(b *gotest.B) {
	parts := []string{"a", "b", "c"}
	for b.B().Loop() {
		_ = strings.Join(parts, ",")
	}
}
