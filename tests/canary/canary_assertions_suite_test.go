package canary_test //nolint:fail-guard // ring 0: raw checks only, never a gotest assertion

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// A poll that started before waitFor is judged by its result, for
// Eventually and Consistently alike, however many ticks it took; a poll
// abandoned past its deadline that panics later is reported to stderr.
func (s *CanaryTestSuite) TestALatePollIsJudgedByItsResult(t *gotest.T) {
	r := s.check(t, "latepolls")
	if strings.Contains(r.stream, "poll still running") {
		t.Errorf("latepolls: a poll started before waitFor was failed as still running")
	}
	if !strings.Contains(r.stream, "gotest: a poll abandoned past its deadline panicked: late boom") {
		t.Errorf("latepolls: no stderr line names the abandoned poll's panic")
	}
}

// A bubble runs on the fake clock and books to the method; what it cannot run
// fails at the call, saying so.
func (s *CanaryTestSuite) TestABubbleRunsOnTheFakeClock(t *gotest.T) {
	r := s.check(t, "bubble")
	src, err := os.ReadFile("testdata/bubble/bubble_suite_test.go")
	if err != nil {
		fatalf(t, "read the bubble fixture: %v", err)
	}
	// at is the fixture's "file:line" of the line the comment ends.
	at := func(comment string) string {
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasSuffix(line, "// "+comment) {
				return "bubble_suite_test.go:" + strconv.Itoa(i+1)
			}
		}
		fatalf(t, "bubble: the fixture marks no line with %q", comment)
		return ""
	}
	suite := "TestBubbleTestSuite/"
	for _, test := range []string{"TestBackoff", "TestSettle", "TestRows/passing"} {
		if !slices.Contains(r.rows, "bubble pass "+suite+test) {
			t.Errorf("bubble: %s did not pass:\n%s", test, streamOutput(r.stream, suite+test))
		}
	}
	if !slices.Contains(r.rows, "bubble fail "+suite+"TestOuterT") {
		t.Errorf("bubble: TestOuterT did not fail: the method's own T was not refused in its bubble")
	}
	count := ""
	for line := range strings.Lines(r.stream) {
		var ev struct{ Action, Test, Key, Value string }
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Action == "attr" && ev.Test == suite+"TestOnlyInside" && ev.Key == protocol.AttrAssertions {
			count = ev.Value
		}
	}
	if count != "3" {
		t.Errorf("bubble: TestOnlyInside gotest.assertions is %q, want \"3\"", count)
	}
	for test, want := range map[string]string{
		"TestFails":         at("fails here") + ": Equal failed",
		"TestEventually":    "Eventually failed after 1h0m0s (61 polls)",
		"TestIt":            at("opens an It") + ": It inside a Bubble: a bubble runs no subtest",
		"TestParallel":      at("calls Parallel") + ": t.Parallel called inside synctest bubble: Run, Parallel and Deadline cannot run in a bubble",
		"TestOuterT":        at("asks the method's T") + ": Context on a T from outside the bubble: inside Bubble use the T its callback is given",
		"TestDeadlock":      at("deadlocks") + ": Bubble deadlock: ",
		"TestSettleOutside": at("settles outside") + ": Settle runs only in a bubble",
	} {
		if out := streamOutput(r.stream, suite+test); !strings.Contains(out, want) {
			t.Errorf("bubble: %s does not say %q:\n%s", test, want, out)
		}
	}
	if out := streamOutput(r.stream, suite+"TestEventually"); strings.Contains(out, "Less failed") {
		t.Errorf("bubble: TestEventually polled an hour on the real clock:\n%s", out)
	}
	if out := streamOutput(r.stream, suite+"TestDeadlock"); !strings.Contains(out, "[chan receive (durable)] at "+at("blocks forever")+"\n") {
		t.Errorf("bubble: TestDeadlock does not name the blocked goroutine's line:\n%s", out)
	}
	abandoned := "TestAbandonTestSuite/TestBlockedOutside"
	if out := streamOutput(r.stream, abandoned); !strings.Contains(out, at("is abandoned")+": Bubble abandoned when the test's context ended") {
		t.Errorf("bubble: %s does not say the bubble was abandoned:\n%s", abandoned, out)
	}
}
