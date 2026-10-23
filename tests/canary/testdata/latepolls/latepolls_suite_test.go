package latepolls_test

import (
	"os"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LatePollsTestSuite: a poll that started before waitFor and returns after
// it is judged by its result, however many ticks it took; one abandoned
// past its deadline that panics later is reported to stderr.
type LatePollsTestSuite struct{}

// wait is waitFor and tick; slowPass returns a quarter tick past the
// deadline, well inside the grace that follows.
const wait = time.Second

func slowPass(*gotest.R) { time.Sleep(wait + wait/4) }

func (s *LatePollsTestSuite) TestConsistentlyPassesALatePoll(t *gotest.T) {
	gotest.Consistently(t, wait, wait, slowPass)
}

func (s *LatePollsTestSuite) TestEventuallyPassesALatePoll(t *gotest.T) {
	gotest.Eventually(t, wait, wait, slowPass)
}

// tick is a twentieth of wait; slowFor is six ticks, well inside the
// waitFor of grace past the deadline.
const (
	tick    = wait / 20
	slowFor = 6 * tick
)

func (s *LatePollsTestSuite) TestConsistentlyPassesAPollOfManyTicks(t *gotest.T) {
	start := time.Now()
	gotest.Consistently(t, wait, tick, func(*gotest.R) {
		if time.Since(start) > wait/2 {
			time.Sleep(time.Until(start.Add(wait + slowFor)))
		}
	})
}

func (s *LatePollsTestSuite) TestEventuallyPassesAFinalPollOfManyTicks(t *gotest.T) {
	start := time.Now()
	gotest.Eventually(t, wait, tick, func(poll *gotest.R) {
		ready := time.Since(start) >= wait
		if ready {
			time.Sleep(slowFor)
		}
		gotest.True(poll, ready)
	})
}

// TestAnAbandonedPollsPanicIsReported reads stderr through a pipe until the
// panic shows, or for 10s, and echoes it. os.Stderr stays the pipe: the
// abandoned poll reads it with no ordering against a restore.
func (s *LatePollsTestSuite) TestAnAbandonedPollsPanicIsReported(t *gotest.T) {
	r, w, err := os.Pipe()
	gotest.NoError(t, err)
	stderr := os.Stderr
	os.Stderr = w
	release := make(chan struct{})
	m := gotest.Record(func(rec *gotest.R) {
		gotest.Eventually(rec, 20*time.Millisecond, 10*time.Millisecond, func(*gotest.R) {
			<-release
			panic("late boom")
		})
	})
	gotest.True(t, m.Failed(), "the hanging poll was not abandoned")
	close(release)
	got := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for !strings.Contains(b.String(), "latepolls_suite_test.go") { // the panicking frame
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		got <- b.String()
	}()
	select {
	case out := <-got:
		_, _ = stderr.WriteString(out)
	case <-time.After(10 * time.Second):
		gotest.Fail(t, "nothing was written to stderr for the abandoned poll's panic")
	}
}
