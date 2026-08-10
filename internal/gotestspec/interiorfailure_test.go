package gotestspec //nolint:stdlib-test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A node can fail on its own account rather than because a behaviour beneath it
// failed: a suite whose AfterAll fails, or a test method that blows its
// configured Timeout. The verdict is attributed to that node, and every child
// under it can still have passed. Reporting only leaves loses it entirely — the
// run exits non-zero with nothing on screen to explain why.

// interiorFailurePackages is a suite whose AfterAll failed after both of its
// behaviours passed.
func interiorFailurePackages() []*Package {
	return []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{{
			Kind:     KindSuite,
			Display:  "Boot",
			Status:   StatusFail,
			Duration: 20 * time.Millisecond,
			Output: []string{
				"--- FAIL: TestBootTestSuite (0.02s)\n",
				"    boot_test.go:14: could not release the database\n",
			},
			Children: []*Node{
				{Kind: KindMethod, Display: "Something", Status: StatusPass, Duration: time.Millisecond},
				{Kind: KindMethod, Display: "Another", Status: StatusPass, Duration: time.Millisecond},
			},
		}},
	}}
}

// bareFailPackages is a parent test that failed via a message-less t.Fail()
// while its only subtest passed: the testing package emits nothing but the
// "--- FAIL:" marker, which filterOutput strips to nothing.
func bareFailPackages() []*Package {
	return []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{{
			Kind:    KindTest,
			Display: "TestGroup",
			Status:  StatusFail,
			Output:  []string{"--- FAIL: TestGroup (0.00s)\n"},
			Children: []*Node{
				{Kind: KindTest, Display: "sub", Status: StatusPass},
			},
		}},
	}}
}

// A verdict comes from status, not prose: requiring surviving output made a
// bare Fail vanish from the count and render an all-green summary beside a red
// exit code.
func TestCollectStats_CountsBareFailInterior(t *testing.T) {
	stats := CollectStats(bareFailPackages())

	gotest.Equal(t, 1, stats.Failed, "a message-less Fail is still a verdict")
}

func TestRenderSummary_BareFailIsVisible(t *testing.T) {
	var buf bytes.Buffer
	RenderSummary(&buf, bareFailPackages(), WithNoColor())
	out := buf.String()

	gotest.Contains(t, out, "of 2 tests failed", "the summary must take the red branch")
	gotest.Contains(t, out, noDiagnosticNote,
		"a counted failure with no output must render the fallback note")
}

func TestRenderTerminal_BareFailIsMarked(t *testing.T) {
	var buf bytes.Buffer
	RenderTerminal(&buf, bareFailPackages(), WithNoColor())

	gotest.Contains(t, buf.String(), noDiagnosticNote,
		"the failing node must carry a visible mark in the tree")
}

// statCounts aggregates the CollectStats expectations so a failure reports
// every mismatched counter at once.
type statCounts struct{ Failed, Passed, Total int }

func TestCollectStats_CountsInteriorNodeFailure(t *testing.T) {
	stats := CollectStats(interiorFailurePackages())

	gotest.Equal(t,
		statCounts{Failed: 1, Passed: 2, Total: stats.Behaviors + stats.Tests},
		statCounts{Failed: stats.Failed, Passed: stats.Passed, Total: stats.Total()})
}

func TestRenderSummary_ReportsInteriorNodeFailure(t *testing.T) {
	var buf bytes.Buffer
	RenderSummary(&buf, interiorFailurePackages(), WithNoColor())
	out := buf.String()

	gotest.Contains(t, out, "could not release the database")
	gotest.Contains(t, out, "Boot")
	gotest.NotContains(t, out, "tests passed (")
}

func TestRenderTerminal_ReportsInteriorNodeFailure(t *testing.T) {
	var buf bytes.Buffer
	RenderTerminal(&buf, interiorFailurePackages(), WithNoColor())
	out := stripANSI(buf.String())

	gotest.Contains(t, out, "could not release the database")
}

// A failed child marks its whole ancestry FAIL. Those ancestors carry only the
// testing package's own "--- FAIL" marker, which is not a verdict of their own —
// counting it would report one failure per level of nesting.
func TestCollectStats_IgnoresFailureInheritedFromAChild(t *testing.T) {
	packages := []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "Boot",
			Status:  StatusFail,
			Output:  []string{"--- FAIL: TestBootTestSuite (0.02s)\n"},
			Children: []*Node{{
				Kind:    KindMethod,
				Display: "Something",
				Status:  StatusFail,
				Output:  []string{"    --- FAIL: TestBootTestSuite/TestSomething (0.01s)\n"},
				Children: []*Node{{
					Kind:    KindBlock,
					Display: "does the thing",
					Status:  StatusFail,
					Output:  []string{"        boot_test.go:9: True failed\n"},
				}},
			}},
		}},
	}}

	stats := CollectStats(packages)
	gotest.Equal(t, 1, stats.Failed, "the leaf only")

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	gotest.Equal(t, 1, strings.Count(buf.String(), "FAIL  example.com/pkg"))
}

// An interior node that merely logged is not a verdict. BeforeEach output lands
// on the method node, so counting any non-marker output would report one extra
// failure for every suite that logs and has a failing behaviour.
func TestCollectStats_IgnoresInteriorLogWhenADescendantFailed(t *testing.T) {
	packages := []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "Boot",
			Status:  StatusFail,
			Output:  []string{"--- FAIL: TestBootTestSuite (0.02s)\n"},
			Children: []*Node{{
				Kind:    KindMethod,
				Display: "Something",
				Status:  StatusFail,
				Output: []string{
					"    --- FAIL: TestBootTestSuite/TestSomething (0.01s)\n",
					"        boot_test.go:7: using db testdb-4711\n",
				},
				Children: []*Node{{
					Kind:    KindBlock,
					Display: "does the thing",
					Status:  StatusFail,
					Output:  []string{"        boot_test.go:9: True failed\n"},
				}},
			}},
		}},
	}}

	stats := CollectStats(packages)
	gotest.Equal(t,
		statCounts{Failed: 1, Passed: 0, Total: stats.Behaviors + stats.Tests},
		statCounts{Failed: stats.Failed, Passed: stats.Passed, Total: stats.Total()},
		"the leaf only")

	// Rendering deliberately uses the weaker hasOwnDiagnostic, not
	// failedOnItsOwn: it cannot tell this stray t.Log apart from a genuine
	// teardown diagnostic without a machine-readable marker on gotest's own
	// verdicts (follow-up work), and showing a stray log line is preferable to
	// ever hiding a real one. So the summary may still list "Something"
	// alongside the leaf — that's the accepted residual. What must not
	// regress is the count above: Failed stays 1 either way.
	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	gotest.Equal(t, 1, strings.Count(buf.String(), "boot_test.go:9: True failed"))
}

// An interior node can carry a genuine diagnostic of its own (AfterAll failed)
// at the same time as a descendant fails independently. The count must still
// attribute the run's one verdict to the leaf, but the teardown diagnostic must
// still reach the screen — it is the only place that says the database was
// never released.
func TestCollectStats_CountsLeafButStillRendersInteriorDiagnostic(t *testing.T) {
	packages := []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "Boot",
			Status:  StatusFail,
			Output: []string{
				"--- FAIL: TestBootTestSuite (0.02s)\n",
				"    boot_test.go:14: could not release the database\n",
			},
			Children: []*Node{
				{Kind: KindMethod, Display: "Something", Status: StatusPass, Duration: time.Millisecond},
				{
					Kind:    KindMethod,
					Display: "Another",
					Status:  StatusFail,
					Output:  []string{"    --- FAIL: TestBootTestSuite/TestAnother (0.01s)\n"},
					Children: []*Node{{
						Kind:    KindBlock,
						Display: "does the thing",
						Status:  StatusFail,
						Output:  []string{"        boot_test.go:9: True failed\n"},
					}},
				},
			},
		}},
	}}

	stats := CollectStats(packages)
	gotest.Equal(t, 1, stats.Failed, "the leaf only")

	var termBuf bytes.Buffer
	RenderTerminal(&termBuf, packages, WithNoColor())
	gotest.Contains(t, stripANSI(termBuf.String()), "could not release the database")

	var summaryBuf bytes.Buffer
	RenderSummary(&summaryBuf, packages, WithNoColor())
	gotest.Contains(t, summaryBuf.String(), "could not release the database")
}
