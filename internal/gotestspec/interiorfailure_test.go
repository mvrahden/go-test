package gotestspec //nolint:stdlib-test

import (
	"bytes"
	"strings"
	"testing"
	"time"
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

func TestCollectStats_CountsInteriorNodeFailure(t *testing.T) {
	stats := CollectStats(interiorFailurePackages())

	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if stats.Passed != 2 {
		t.Errorf("Passed = %d, want 2", stats.Passed)
	}
	if stats.Total() != stats.Behaviors+stats.Tests {
		t.Errorf("Passed+Failed+Skipped = %d, want it to equal Behaviors+Tests = %d",
			stats.Total(), stats.Behaviors+stats.Tests)
	}
}

func TestRenderSummary_ReportsInteriorNodeFailure(t *testing.T) {
	var buf bytes.Buffer
	RenderSummary(&buf, interiorFailurePackages(), WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "could not release the database") {
		t.Errorf("expected the node's own diagnostic in the summary, got:\n%s", out)
	}
	if !strings.Contains(out, "Boot") {
		t.Errorf("expected the failing node to be named, got:\n%s", out)
	}
	if strings.Contains(out, "tests passed (") {
		t.Errorf("expected a failure summary, not an all-passed line:\n%s", out)
	}
}

func TestRenderTerminal_ReportsInteriorNodeFailure(t *testing.T) {
	var buf bytes.Buffer
	RenderTerminal(&buf, interiorFailurePackages(), WithNoColor())
	out := stripANSI(buf.String())

	if !strings.Contains(out, "could not release the database") {
		t.Errorf("expected the node's own diagnostic in the tree, got:\n%s", out)
	}
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
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (the leaf only)", stats.Failed)
	}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	if got := strings.Count(buf.String(), "FAIL  example.com/pkg"); got != 1 {
		t.Errorf("reported %d failures, want 1:\n%s", got, buf.String())
	}
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
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (the leaf only)", stats.Failed)
	}
	if stats.Total() != stats.Behaviors+stats.Tests {
		t.Errorf("Passed+Failed+Skipped = %d, want it to equal Behaviors+Tests = %d",
			stats.Total(), stats.Behaviors+stats.Tests)
	}

	// Rendering deliberately uses the weaker hasOwnDiagnostic, not
	// failedOnItsOwn: it cannot tell this stray t.Log apart from a genuine
	// teardown diagnostic without a machine-readable marker on gotest's own
	// verdicts (follow-up work), and showing a stray log line is preferable to
	// ever hiding a real one. So the summary may still list "Something"
	// alongside the leaf — that's the accepted residual. What must not
	// regress is the count above: Failed stays 1 either way.
	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	if got := strings.Count(buf.String(), "boot_test.go:9: True failed"); got != 1 {
		t.Errorf("expected the leaf's own diagnostic exactly once, got %d:\n%s", got, buf.String())
	}
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
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (the leaf only)", stats.Failed)
	}

	var termBuf bytes.Buffer
	RenderTerminal(&termBuf, packages, WithNoColor())
	if termOut := stripANSI(termBuf.String()); !strings.Contains(termOut, "could not release the database") {
		t.Errorf("expected the interior node's own diagnostic in the tree, got:\n%s", termOut)
	}

	var summaryBuf bytes.Buffer
	RenderSummary(&summaryBuf, packages, WithNoColor())
	if summaryOut := summaryBuf.String(); !strings.Contains(summaryOut, "could not release the database") {
		t.Errorf("expected the interior node's own diagnostic in the summary, got:\n%s", summaryOut)
	}
}
