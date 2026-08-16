package gotestspec_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DurationTestSuite covers what the tree says about time. Every row is an
// interval — the wall clock a thing occupied — and never a sum of its children,
// which is an integral over occupancy and reports more time than the clock did
// as soon as anything runs in parallel.
type DurationTestSuite struct{}

var base = time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)

// at builds a node bracketed from offset to offset+span, measuring measured.
func at(display string, offset, span, measured time.Duration, kids ...*gotestspec.Node) *gotestspec.Node {
	return &gotestspec.Node{
		Display:  display,
		Status:   gotestspec.StatusPass,
		Duration: measured,
		Start:    base.Add(offset),
		End:      base.Add(offset + span),
		Children: kids,
	}
}

// parked marks a node as one that called t.Parallel, which is what makes its
// own bracket untrustworthy in both directions.
func parked(n *gotestspec.Node) *gotestspec.Node {
	n.Paused = true
	return n
}

func (s *DurationTestSuite) TestParkedNodeReportsWhatItExecuted(t *gotest.T) {
	t.When("a parallel leaf was parked before it ran", func(w *gotest.T) {
		// Registered at once, resumed 50ms later, then done in a millisecond.
		// The wait belongs to whatever set the concurrency, not to the test.
		leaf := parked(at("matches correctly", 0, 51*time.Millisecond, time.Millisecond))

		w.It("charges only its own execution", func(it *gotest.T) {
			gotest.Equal(it, time.Millisecond, gotestspec.EffectiveDuration(leaf),
				"a leaf that did nothing must not be billed for queueing")
		})
	})

	t.When("a parked method's report was flushed late", func(w *gotest.T) {
		// Measured against a real stream: the method resumed at 5.1s, ran for
		// 300ms, and had its terminal event held behind a 60s sibling — so its
		// bracket reads 60.7s for 300ms of work. Its children keep clean
		// timestamps; only the parked node's own end is delayed.
		method := parked(at("PanicInEachAfterRecordedFailure", 0, 60783*time.Millisecond, 300*time.Millisecond,
			at("an Each entry records a failure and then panics", 5107*time.Millisecond, 300*time.Millisecond, 300*time.Millisecond),
		))

		w.It("ignores its bracket entirely", func(it *gotest.T) {
			gotest.Equal(it, 300*time.Millisecond, gotestspec.EffectiveDuration(method),
				"a 300ms method must not read as a minute because a sibling held the flush")
		})

		w.It("leaves the child on its own clean bracket", func(it *gotest.T) {
			gotest.Equal(it, 300*time.Millisecond, gotestspec.EffectiveDuration(method.Children[0]))
		})
	})

	t.When("its measure rounds below a child's", func(w *gotest.T) {
		// go test reports a parked node's measure to 10ms while a child's
		// bracket is exact, so a 15ms method can read 10ms beside its own 15ms
		// child. A child runs inside its parent, so the parent is floored by it.
		method := parked(at("GraceKill", 0, 9*time.Second, 10*time.Millisecond,
			at("using GraceKill strategy", 0, 15*time.Millisecond, 15*time.Millisecond),
		))

		w.It("never reads shorter than what it contains", func(it *gotest.T) {
			gotest.Equal(it, 15*time.Millisecond, gotestspec.EffectiveDuration(method))
		})
	})

	t.When("a parked child's measure rounds above its suite's bracket", func(w *gotest.T) {
		// The same rounding seen from the other side: the suite is not parked,
		// so it reports an exact bracket, which its child can still exceed.
		suite := at("Spec", 0, 229*time.Millisecond, 0,
			parked(at("DetermineTestSuiteHarness", 0, 229*time.Millisecond, 230*time.Millisecond)),
		)

		w.It("lifts the suite to its child", func(it *gotest.T) {
			gotest.Equal(it, 230*time.Millisecond, gotestspec.EffectiveDuration(suite))
		})
	})

	t.When("a parked node is unioned with others", func(w *gotest.T) {
		// Its Duration says how long it ran but neither endpoint says when, so
		// it contributes nothing rather than a window in the wrong place.
		pkg := &gotestspec.Package{Path: "p", Nodes: []*gotestspec.Node{
			at("Alpha", 0, 100*time.Millisecond, 100*time.Millisecond),
			parked(at("Beta", 0, 60*time.Second, time.Millisecond)),
		}}

		w.It("is left out rather than placed wrongly", func(it *gotest.T) {
			gotest.Equal(it, 100*time.Millisecond, gotestspec.PackageDuration(pkg))
		})
	})
}

func (s *DurationTestSuite) TestContainerReportsItsBracket(t *gotest.T) {
	t.When("its children ran in parallel", func(w *gotest.T) {
		// go test stops a parent's clock when it returns, so the parent measures
		// nothing, while its children together claim more than the clock passed.
		suite := at("Gotestast", 0, 58*time.Millisecond, 0,
			parked(at("DetermineFixtureHarness", 0, 58*time.Millisecond, 58*time.Millisecond)),
			parked(at("FixtureValidation", 0, 50*time.Millisecond, 50*time.Millisecond)),
			parked(at("ClassifyLocalFields", 0, 52*time.Millisecond, 52*time.Millisecond)),
		)

		w.It("reports the interval, not the sum", func(it *gotest.T) {
			gotest.Equal(it, 58*time.Millisecond, gotestspec.EffectiveDuration(suite),
				"three 50ms methods sharing 58ms of clock cost 58ms, not 160ms")
		})
	})

	t.When("it held time its children never report", func(w *gotest.T) {
		// The shape of a When that drives a subprocess: the work happens in the
		// container and the leaves only assert on what it produced.
		when := at("the setup never returns", 0, 60*time.Second, 60*time.Second,
			at("names the budget", 60*time.Second, 0, 0),
			at("fails the run", 60*time.Second, 0, 0),
		)

		w.It("still reports what it cost", func(it *gotest.T) {
			gotest.Equal(it, 60*time.Second, gotestspec.EffectiveDuration(when))
		})
	})

	t.When("the tree carries no timestamps", func(w *gotest.T) {
		// A static spec or a hand-built fixture. The subtree is then bounded
		// below by both the node's own measure and its children's.
		held := &gotestspec.Node{Display: "held", Duration: 5 * time.Second,
			Children: []*gotestspec.Node{{Display: "leaf", Status: gotestspec.StatusPass}}}
		parallel := &gotestspec.Node{Display: "parallel",
			Children: []*gotestspec.Node{
				{Display: "a", Duration: 500 * time.Millisecond},
				{Display: "b", Duration: 500 * time.Millisecond},
			}}

		w.It("falls back to whichever bound is larger", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Second, gotestspec.EffectiveDuration(held))
			gotest.Equal(it, time.Second, gotestspec.EffectiveDuration(parallel))
		})
	})
}

func (s *DurationTestSuite) TestPackageUnionsWhatItsSuitesOccupied(t *gotest.T) {
	t.When("its suites overlapped", func(w *gotest.T) {
		// A package is not something that executes — the runner dispatches its
		// suites among every other package's — so it is worth exactly the clock
		// during which one of them was running.
		pkg := &gotestspec.Package{Path: "p", Nodes: []*gotestspec.Node{
			at("Alpha", 0, 100*time.Millisecond, 100*time.Millisecond,
				at("one", 0, 100*time.Millisecond, 100*time.Millisecond)),
			at("Beta", 50*time.Millisecond, 100*time.Millisecond, 100*time.Millisecond,
				at("two", 50*time.Millisecond, 100*time.Millisecond, 100*time.Millisecond)),
		}}

		w.It("counts the overlap once", func(it *gotest.T) {
			gotest.Equal(it, 150*time.Millisecond, gotestspec.PackageDuration(pkg),
				"two 100ms suites overlapping by 50ms occupied 150ms of clock")
		})
	})
}

func (s *DurationTestSuite) TestTotalSkipsTheGapBetweenRuns(t *gotest.T) {
	t.When("a tree holds results recorded hours apart", func(w *gotest.T) {
		// The interval between two runs is nobody's cost. Unioning the windows
		// rather than spanning them keeps the number honest without having to
		// know which run each result came from.
		morning := &gotestspec.Package{Path: "a", Nodes: []*gotestspec.Node{
			at("Alpha", 0, time.Second, time.Second, at("one", 0, time.Second, time.Second))}}
		afternoon := &gotestspec.Package{Path: "b", Nodes: []*gotestspec.Node{
			at("Beta", 4*time.Hour, 2*time.Second, 2*time.Second,
				at("two", 4*time.Hour, 2*time.Second, 2*time.Second))}}

		w.It("totals the time something was running", func(it *gotest.T) {
			gotest.Equal(it, 3*time.Second,
				gotestspec.TotalDuration([]*gotestspec.Package{morning, afternoon}))
		})
	})
}

func (s *DurationTestSuite) TestRenderedTreeCarriesTheEffectiveTime(t *gotest.T) {
	t.When("a parallel suite is rendered", func(w *gotest.T) {
		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, []*gotestspec.Package{{Path: "p", Nodes: []*gotestspec.Node{
			at("Parallel", 0, 300*time.Millisecond, 0,
				at("first", 0, 300*time.Millisecond, 300*time.Millisecond),
				at("second", 0, 300*time.Millisecond, 300*time.Millisecond),
			)}}}, gotestspec.WithNoColor())
		out := buf.String()

		w.It("shows the clock the suite occupied", func(it *gotest.T) {
			gotest.Contains(it, lineWith(out, "Parallel"), "(300ms)",
				"the rendered row must not sum concurrent children:\n%s", out)
		})
	})

	t.When("the tree is rendered as a specification", func(w *gotest.T) {
		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, []*gotestspec.Package{{Path: "p", Nodes: []*gotestspec.Node{
			at("Hangs", 0, 60*time.Second, 0, at("leaf", 0, 0, 0))}}},
			gotestspec.WithNoColor(), gotestspec.WithoutVerdicts())
		out := buf.String()

		w.It("quotes no duration at all", func(it *gotest.T) {
			gotest.NotContains(it, out, "60.0s",
				"a spec has executed nothing and must claim no time:\n%s", out)
		})
	})
}

// lineWith returns the rendered row carrying substr, so an assertion can name
// the row it means instead of searching the whole tree.
func lineWith(output, substr string) string {
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}
