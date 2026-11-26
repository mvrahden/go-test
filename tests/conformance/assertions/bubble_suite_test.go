package assertions_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BubbleTestSuite: gotest.Bubble runs its callback on the fake clock, books
// it to the method and refuses what a bubble cannot run.
type BubbleTestSuite struct {
	run testkit.Run
}

func (s *BubbleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, SetupTimeout: time.Minute}
}

const (
	bubblePkg   = "../testdata/bubble/"
	bubbleFile  = bubblePkg + "bubble_suite_test.go"
	bubbleSuite = "TestBubbleTestSuite/"
)

func (s *BubbleTestSuite) BeforeAll(t *gotest.T) {
	var err error
	s.run, err = testkit.RunPackage(t.Context(), t.TempDir(), bubblePkg)
	gotest.NoError(t, err)
}

func (s *BubbleTestSuite) AfterAll(t *gotest.T) {}

// output is what the run printed for test.
func (s *BubbleTestSuite) output(test string) string {
	var b strings.Builder
	for _, ev := range s.run.Events {
		if ev.Action == "output" && ev.Test == test {
			b.WriteString(ev.Output)
		}
	}
	return b.String()
}

// at is the fixture's "file:line: " for the line marked by the comment.
func at(t *gotest.T, comment string) string {
	src, err := os.ReadFile(bubbleFile)
	gotest.NoError(t, err)
	for i, line := range strings.Split(string(src), "\n") {
		if strings.HasSuffix(line, "// "+comment) {
			return fmt.Sprintf("bubble_suite_test.go:%d: ", i+1)
		}
	}
	gotest.Fail(t, "the fixture marks no line with "+comment)
	return ""
}

func (s *BubbleTestSuite) TestBubbles(t *gotest.T) {
	t.When("a method waits an hour of backoff inside a Bubble", func(w *gotest.T) {
		w.It("passes in less than a second of real time", func(it *gotest.T) {
			gotest.Equal(it, "pass", s.run.Verdict(bubbleSuite+"TestBackoff"), s.output(bubbleSuite+"TestBackoff"))
		})
	})
	t.When("a leaf asserts only inside a Bubble, on its T and its *testing.T", func(w *gotest.T) {
		w.It("reports the two assertions as its count", func(it *gotest.T) {
			n, ok := s.run.Attr(bubbleSuite+"TestOnlyInside", protocol.AttrAssertions)
			gotest.True(it, ok, "no count reported")
			gotest.Equal(it, "2", n)
		})
	})
	t.When("an assertion fails inside a Bubble", func(w *gotest.T) {
		w.It("fails the method at the assertion's line", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestFails"))
			gotest.Contains(it, s.output(bubbleSuite+"TestFails"), at(it, "fails here")+"Equal failed")
		})
	})
	t.When("an Each row's Bubble fails", func(w *gotest.T) {
		w.It("fails the row, and the next row runs and passes", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestRows/failing"))
			gotest.Equal(it, "pass", s.run.Verdict(bubbleSuite+"TestRows/passing"))
		})
	})
	t.When("Eventually polls for an hour inside a Bubble and no poll passes", func(w *gotest.T) {
		w.It("fails after an hour of fake time, in less than a second of real time", func(it *gotest.T) {
			out := s.output(bubbleSuite + "TestEventually/hour")
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestEventually/hour"))
			gotest.Contains(it, out, "Eventually failed after 1h0m0s (61 polls)")
			gotest.NotContains(it, out, "Less failed")
		})
	})
	t.When("a goroutine in a Bubble writes a value as it returns", func(w *gotest.T) {
		w.It("the callback reads the value after Settle", func(it *gotest.T) {
			gotest.Equal(it, "pass", s.run.Verdict(bubbleSuite+"TestSettle"), s.output(bubbleSuite+"TestSettle"))
		})
	})
}

func (s *BubbleTestSuite) TestWhatABubbleRefuses(t *gotest.T) {
	t.When("an It is opened inside a Bubble", func(w *gotest.T) {
		w.It("fails at the It's line, saying a bubble runs no subtest", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestIt"))
			gotest.Contains(it, s.output(bubbleSuite+"TestIt"), at(it, "opens an It")+"It inside a Bubble: a bubble runs no subtest")
		})
	})
	t.When("the bubble's *testing.T calls Parallel", func(w *gotest.T) {
		w.It("fails at the call, saying Parallel cannot run in a bubble", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestParallel"))
			gotest.Contains(it, s.output(bubbleSuite+"TestParallel"), at(it, "calls Parallel")+"t.Parallel called inside synctest bubble: Run, Parallel and Deadline cannot run in a bubble")
		})
	})
	t.When("a Bubble's callback asks the method's own T for its Context", func(w *gotest.T) {
		w.It("fails at the call, saying to use the T the callback is given", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestOuterT"))
			gotest.Contains(it, s.output(bubbleSuite+"TestOuterT"), at(it, "asks the method's T")+"Context on a T from outside the bubble: inside Bubble use the T its callback is given")
		})
	})
	t.When("every goroutine in a Bubble blocks on a channel with no timer due", func(w *gotest.T) {
		w.It("fails at the Bubble, naming the deadlock and the blocked goroutine's line", func(it *gotest.T) {
			out := s.output(bubbleSuite + "TestDeadlock")
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestDeadlock"))
			gotest.Contains(it, out, at(it, "deadlocks")+"Bubble deadlock: ")
			blocked := regexp.QuoteMeta(strings.TrimSuffix(at(it, "blocks forever"), ": "))
			gotest.Regexp(it, `goroutine \d+ \[chan receive \(durable\)\] at `+blocked+`\n`, out)
		})
	})
	t.When("Settle is given the method's T", func(w *gotest.T) {
		w.It("fails at the call, saying Settle runs only in a bubble", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(bubbleSuite+"TestSettleOutside"))
			gotest.Contains(it, s.output(bubbleSuite+"TestSettleOutside"), at(it, "settles outside")+"Settle runs only in a bubble")
		})
	})
}

func (s *BubbleTestSuite) TestABubbleAndTheTimeout(t *gotest.T) {
	t.When("a Bubble waits on a mutex held outside it past the Timeout", func(w *gotest.T) {
		w.It("fails at the Bubble, saying the bubble was abandoned", func(it *gotest.T) {
			test := "TestAbandonTestSuite/TestBlockedOutside"
			gotest.Equal(it, "fail", s.run.Verdict(test))
			gotest.Contains(it, s.output(test), at(it, "is abandoned")+"Bubble abandoned when the test's context ended")
		})
	})
}
