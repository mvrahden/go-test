package testpkg

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchEndsEarlyTestSuite has benchmarks that end by failure and by panic.
type BenchEndsEarlyTestSuite struct{}

func (s *BenchEndsEarlyTestSuite) AfterEach(t *gotest.T) { fmt.Println("MARK:aftereach ran") }

func (s *BenchEndsEarlyTestSuite) BenchmarkFails(b *gotest.B) {
	gotest.Fail(b, "benchmark fails on purpose")
}

func (s *BenchEndsEarlyTestSuite) BenchmarkPanics(b *gotest.B) {
	panic("benchmark panics on purpose")
}
