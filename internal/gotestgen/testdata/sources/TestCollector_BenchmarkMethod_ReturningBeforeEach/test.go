package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type benchCtx struct{ val string }

type ReturningBenchTestSuite struct{}

func (s *ReturningBenchTestSuite) BeforeEach(t *gotest.T) *benchCtx   { return &benchCtx{} }
func (s *ReturningBenchTestSuite) TestOne(t *gotest.T, ctx *benchCtx) {}
func (s *ReturningBenchTestSuite) BenchmarkParse(b *gotest.B) {
	for b.Loop() {
	}
}
