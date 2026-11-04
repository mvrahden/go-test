package lintescapes

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// harness has a B method of its own; it is no gotest.B.
type harness struct{ b *testing.B }

func (h *harness) B() *testing.B { return h.b }

type EscapesTestSuite struct{}

func (s *EscapesTestSuite) BenchmarkEscapes(b *gotest.B) {
	for b.B().Loop() {
		gotest.Equal(b.B(), 4, 2*2)
	}
	b.B().Helper()
	b.B().Cleanup(func() {})
	_ = b.B().N
}

func (s *EscapesTestSuite) BenchmarkSleepThroughEscape(b *gotest.B) {
	for b.B().Loop() {
		time.Sleep(time.Millisecond)
	}
}

func (s *EscapesTestSuite) BenchmarkOwnB(b *gotest.B) {
	h := &harness{b: nil}
	h.B().Helper()
	h.B().ReportAllocs()
	for b.Loop() {
	}
}

type SeedsTestSuite struct{}

func (s *SeedsTestSuite) FuzzEscapes(f *gotest.F) {
	f.F().Add("s")
	f.Fuzz(func(t *gotest.T, in string) {
		gotest.NotEmpty(t, in)
	})
}
