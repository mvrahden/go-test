package orphanwarned

import "github.com/mvrahden/go-test/pkg/gotest"

// timer is a helper a suite calls.
type timer struct{}

func (tm *timer) BenchmarkTick(b *gotest.B) {}

type RealTestSuite struct{ timer timer }

func (s *RealTestSuite) TestRuns(t *gotest.T) {}

func (s *RealTestSuite) BenchmarkTicks(b *gotest.B) { s.timer.BenchmarkTick(b) }
