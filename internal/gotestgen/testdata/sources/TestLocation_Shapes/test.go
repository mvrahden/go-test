package testpkg

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ShapesTestSuite fails an assertion through each testing type gotest
// accepts. Every failing line carries its own marker for the test to find.
type ShapesTestSuite struct{}

func (s *ShapesTestSuite) TestStd(t *testing.T) {
	gotest.Fail(t, "std") // LINE:std
}

func (s *ShapesTestSuite) TestStdHelper(t *testing.T) {
	stdHelper(t) // LINE:std-helper-call
}

func stdHelper(t *testing.T) {
	gotest.Fail(t, "std-helper") // LINE:std-helper
}

func (s *ShapesTestSuite) TestSkip(t *gotest.T) {
	t.Skipf("skipped") // LINE:skip
}

func (s *ShapesTestSuite) BenchmarkFail(b *gotest.B) {
	for b.Loop() {
		gotest.Fail(b, "bench") // LINE:bench
	}
}

func (s *ShapesTestSuite) BenchmarkHelper(b *gotest.B) {
	for b.Loop() {
		benchHelper(b) // LINE:bench-helper-call
	}
}

func benchHelper(b *gotest.B) {
	gotest.Fail(b, "bench-helper") // LINE:bench-helper
}

func (s *ShapesTestSuite) FuzzBody(f *gotest.F) {
	f.Add(1)
	f.Fuzz(func(t *gotest.T, n int) {
		gotest.Fail(t, "fuzz") // LINE:fuzz
	})
}

func (s *ShapesTestSuite) FuzzOnF(f *gotest.F) {
	gotest.Fail(f, "on-f") // LINE:on-f
	f.Fuzz(func(t *gotest.T, n int) {})
}
