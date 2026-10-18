package parallelpanic

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelPanicTestSuite runs its methods in parallel; TestPanics panics
// while TestSlow is still running.
type ParallelPanicTestSuite struct {
	Track *TrackFixture
}

func (s *ParallelPanicTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ParallelPanicTestSuite) AfterEach(t *gotest.T) { mark("aftereach " + t.T().Name()) }
func (s *ParallelPanicTestSuite) AfterAll(t *gotest.T)  { mark("afterall") }

func (s *ParallelPanicTestSuite) TestPanics(t *gotest.T) {
	time.Sleep(200 * time.Millisecond)
	panic("the method panicked")
}

func (s *ParallelPanicTestSuite) TestSlow(t *gotest.T) {
	mark("slow started")
	time.Sleep(2 * time.Second)
	mark("slow finished")
}
