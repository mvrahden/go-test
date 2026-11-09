//go:build linux

package gotestruntime_test

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunnerGoneTestSuite: a suite process whose runner's end of the channel
// closes stops as the runner would have stopped it, then ends what its tests
// started in its process group, nobody else being left to.
type RunnerGoneTestSuite struct{}

func (s *RunnerGoneTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

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

func (s *RunnerGoneTestSuite) TestTheChannelsEndDuringARequestedStop(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, "TestRunnerGoneTestSuite", "TestTheChannelsEndDuringARequestedStop", child{mode: holdOnContext, slow: 2 * time.Second})
	r.awaitMarker(t, "holding")
	gotest.NoError(t, r.link.Stop())
	r.awaitMarker(t, "AfterAll began")
	r.link.Close()
	r.wait(t)

	t.When("the runner's end of the channel closes while a stop it asked for runs AfterAll", func(w *gotest.T) {
		w.It("lets AfterAll run to its end, then releases the fixture", func(it *gotest.T) {
			gotest.Equal(it, []string{"fixture BeforeAll", "holding", "context ended",
				"AfterAll began", "suite AfterAll", "fixture AfterAll"}, r.markers())
			gotest.NotContains(it, r.output(), "forced")
		})
	})
}

// The kernel sends the runner's death signal as the runner's thread that
// started the suite exits, which can come before its last thread closes the
// channel: the stop the signal began settles first, and the end comes after.
func (s *RunnerGoneTestSuite) TestTheChannelsEndAfterTheDeathSignal(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, "TestRunnerGoneTestSuite", "TestTheChannelsEndAfterTheDeathSignal", child{mode: holdGrandchild})
	r.awaitMarker(t, "holding")
	sleeper := r.grandchild()
	gotest.Greater(t, sleeper, 0, "the method started no process")
	gotest.NoError(t, syscall.Kill(r.cmd.Process.Pid, syscall.SIGTERM))
	r.awaitMarker(t, "fixture AfterAll")
	// Past the settled stop, well within the wait for the end.
	time.Sleep(300 * time.Millisecond)
	r.link.Close()
	r.wait(t)

	t.When("the runner's death signal stops a suite process whose method has started a process, and the channel ends after that stop settled", func(w *gotest.T) {
		w.It("ends that process with itself, though it ignores SIGTERM", func(it *gotest.T) {
			gotest.Eventually(it, 3*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(sleeper), "the process the method started outlived the suite process")
			})
		})
	})
}

// processGone reports whether pid has exited: no such process, or one only
// waiting to be reaped.
func processGone(pid int) bool {
	if syscall.Kill(pid, 0) == syscall.ESRCH {
		return true
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	end := strings.LastIndexByte(string(stat), ')')
	return end > 0 && end+2 < len(stat) && stat[end+2] == 'Z'
}
