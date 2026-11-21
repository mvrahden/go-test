package gotestrunner_test

import (
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CompileSpanTestSuite: the compile stage's span runs from its start to
// its last outcome.
type CompileSpanTestSuite struct{}

func (s *CompileSpanTestSuite) TestSpan(t *gotest.T) {
	// The stage takes a second per outcome: the clock moves after each.
	var clock atomic.Int64
	now := func() time.Time { return time.Unix(100+clock.Load(), 0) }
	in := make(chan gotestrunner.CompileOutcome)
	out, span := gotestrunner.ExportSpanOf(in, now)
	gotest.Go(t, func() {
		for _, pkg := range []string{"a", "b"} {
			in <- gotestrunner.CompileOutcome{Package: pkg}
			clock.Add(1)
		}
		close(in)
	})
	t.It("is zero until the stage has ended", func(it *gotest.T) {
		gotest.Zero(it, span())
	})
	var got []string
	for o := range out {
		got = append(got, o.Package)
	}
	t.It("passes every outcome on, in order", func(it *gotest.T) {
		gotest.Equal(it, []string{"a", "b"}, got)
	})
	t.It("is the time from the call to the channel's close", func(it *gotest.T) {
		gotest.Equal(it, 2*time.Second, span())
	})
}
