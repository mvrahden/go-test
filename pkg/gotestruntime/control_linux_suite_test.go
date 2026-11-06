//go:build linux

package gotestruntime_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunnerGoneTestSuite: a suite process whose runner's end of the channel
// closes while a method holds stops as the runner would have stopped it, then
// ends what its tests started in its process group, nobody else being left to.
type RunnerGoneTestSuite struct{}

func (s *RunnerGoneTestSuite) BeforeEach(t *gotest.T) *linkRun { return &linkRun{} }

func (s *RunnerGoneTestSuite) AfterEach(t *gotest.T, r *linkRun) { r.reap() }

func (s *RunnerGoneTestSuite) TestTheChannelsEndWhileAMethodHolds(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, "TestRunnerGoneTestSuite", "TestTheChannelsEndWhileAMethodHolds", child{mode: holdGrandchild})
	r.awaitMarker(t, "holding")
	sleeper := r.grandchild()
	gotest.Greater(t, sleeper, 0, "the method started no process")
	r.link.Close()
	r.wait(t)

	t.When("the runner's end of the channel closes while a method holds and has started a process", func(w *gotest.T) {
		w.It("ends the method's context, runs AfterAll, then releases the fixture", func(it *gotest.T) {
			gotest.Equal(it, []string{"fixture BeforeAll", "holding", "context ended",
				"AfterAll began", "suite AfterAll", "fixture AfterAll"}, r.markers())
		})
		w.It("ends that process with itself, though it ignores SIGTERM", func(it *gotest.T) {
			gotest.Eventually(it, 3*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(sleeper), "the process the method started outlived the suite process")
			})
		})
	})
}
