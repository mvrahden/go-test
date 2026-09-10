// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// InteriorFailureTestSuite covers verdicts attributed to a node itself rather
// than to a behaviour beneath it: a suite whose AfterAll fails, or a method
// that blows its Timeout, with every child passing. Reporting only leaves
// loses it entirely: the run exits non-zero with nothing on screen to say why.
type InteriorFailureTestSuite struct{}

func (s *InteriorFailureTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type interiorCtx struct{}

func (s *InteriorFailureTestSuite) BeforeEach(t *gotest.T) *interiorCtx { return &interiorCtx{} }

// interiorFailurePackages is a suite whose AfterAll failed after both of its
// behaviours passed.
func interiorFailurePackages() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{{
			Kind:     gotestspec.KindSuite,
			Display:  "Boot",
			Status:   gotestspec.StatusFail,
			Duration: 20 * time.Millisecond,
			Output: []string{
				"--- FAIL: TestBootTestSuite (0.02s)\n",
				"    boot_test.go:14: could not release the database\n",
			},
			Children: []*gotestspec.Node{
				{Kind: gotestspec.KindMethod, Display: "Something", Status: gotestspec.StatusPass, Duration: time.Millisecond},
				{Kind: gotestspec.KindMethod, Display: "Another", Status: gotestspec.StatusPass, Duration: time.Millisecond},
			},
		}},
	}}
}

// bareFailPackages is a parent test that failed via a message-less t.Fail()
// while its only subtest passed: the testing package emits nothing but the
// "--- FAIL:" marker, which output filtering strips to nothing.
func bareFailPackages() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindTest,
			Display: "TestGroup",
			Status:  gotestspec.StatusFail,
			Output:  []string{"--- FAIL: TestGroup (0.00s)\n"},
			Children: []*gotestspec.Node{
				{Kind: gotestspec.KindTest, Display: "sub", Status: gotestspec.StatusPass},
			},
		}},
	}}
}

func summaryOf(packages []*gotestspec.Package) string {
	var buf bytes.Buffer
	gotestspec.RenderSummary(&buf, packages, gotestspec.WithNoColor())
	return buf.String()
}

func terminalOf(packages []*gotestspec.Package) string {
	var buf bytes.Buffer
	gotestspec.RenderTerminal(&buf, packages, gotestspec.WithNoColor())
	return gotestspec.ExportStripANSI(buf.String())
}

// A verdict comes from status, not prose: requiring surviving output made a
// bare Fail vanish from the count and render an all-green summary beside a red
// exit code.
func (s *InteriorFailureTestSuite) TestCollectStats_CountsBareFailInterior(t *gotest.T, _ *interiorCtx) {
	stats := gotestspec.CollectStats(bareFailPackages())
	mustEq(t, "failed (a message-less Fail is still a verdict)", stats.Failed, 1)
}

func (s *InteriorFailureTestSuite) TestRenderSummary_BareFailIsVisible(t *gotest.T, _ *interiorCtx) {
	out := summaryOf(bareFailPackages())
	mustContain(t, out, "of 2 tests failed", "the summary must take the red branch")
	mustContain(t, out, gotestspec.ExportNoDiagnosticNote, "a counted failure with no output renders the fallback note")
}

func (s *InteriorFailureTestSuite) TestRenderTerminal_BareFailIsMarked(t *gotest.T, _ *interiorCtx) {
	mustContain(t, terminalOf(bareFailPackages()), gotestspec.ExportNoDiagnosticNote, "the failing node carries a visible mark in the tree")
}

// statCounts aggregates the CollectStats expectations so a failure reports
// every mismatched counter at once.
type statCounts struct{ Failed, Passed, Total int }

func countsOf(stats gotestspec.Stats) statCounts {
	return statCounts{Failed: stats.Failed, Passed: stats.Passed, Total: stats.Total()}
}

func (s *InteriorFailureTestSuite) TestCollectStats_CountsInteriorNodeFailure(t *gotest.T, _ *interiorCtx) {
	stats := gotestspec.CollectStats(interiorFailurePackages())
	mustEq(t, "counts", countsOf(stats), statCounts{Failed: 1, Passed: 2, Total: stats.Behaviors + stats.Tests})
}

func (s *InteriorFailureTestSuite) TestRenderSummary_ReportsInteriorNodeFailure(t *gotest.T, _ *interiorCtx) {
	out := summaryOf(interiorFailurePackages())
	mustContain(t, out, "could not release the database", "the teardown diagnostic is shown")
	mustContain(t, out, "Boot", "the failing suite is named")
	mustNotContain(t, out, "tests passed (", "the run is not reported green")
}

func (s *InteriorFailureTestSuite) TestRenderTerminal_ReportsInteriorNodeFailure(t *gotest.T, _ *interiorCtx) {
	mustContain(t, terminalOf(interiorFailurePackages()), "could not release the database", "the teardown diagnostic is shown")
}

// A failed child marks its whole ancestry FAIL. Those ancestors carry only the
// testing package's own "--- FAIL" marker, which is not a verdict of their own;
// counting it would report one failure per level of nesting.
func (s *InteriorFailureTestSuite) TestCollectStats_IgnoresFailureInheritedFromAChild(t *gotest.T, _ *interiorCtx) {
	packages := []*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Boot",
			Status:  gotestspec.StatusFail,
			Output:  []string{"--- FAIL: TestBootTestSuite (0.02s)\n"},
			Children: []*gotestspec.Node{{
				Kind:    gotestspec.KindMethod,
				Display: "Something",
				Status:  gotestspec.StatusFail,
				Output:  []string{"    --- FAIL: TestBootTestSuite/TestSomething (0.01s)\n"},
				Children: []*gotestspec.Node{{
					Kind:    gotestspec.KindBlock,
					Display: "does the thing",
					Status:  gotestspec.StatusFail,
					Output:  []string{"        boot_test.go:9: True failed\n"},
				}},
			}},
		}},
	}}

	mustEq(t, "failed (the leaf only)", gotestspec.CollectStats(packages).Failed, 1)
	mustEq(t, "FAIL lines in the summary", strings.Count(summaryOf(packages), "FAIL  example.com/pkg"), 1)
}

// An interior node that merely logged is not a verdict. BeforeEach output lands
// on the method node, so counting any non-marker output would report one extra
// failure for every suite that logs and has a failing behaviour.
func (s *InteriorFailureTestSuite) TestCollectStats_IgnoresInteriorLogWhenADescendantFailed(t *gotest.T, _ *interiorCtx) {
	packages := []*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Boot",
			Status:  gotestspec.StatusFail,
			Output:  []string{"--- FAIL: TestBootTestSuite (0.02s)\n"},
			Children: []*gotestspec.Node{{
				Kind:    gotestspec.KindMethod,
				Display: "Something",
				Status:  gotestspec.StatusFail,
				Output: []string{
					"    --- FAIL: TestBootTestSuite/TestSomething (0.01s)\n",
					"        boot_test.go:7: using db testdb-4711\n",
				},
				Children: []*gotestspec.Node{{
					Kind:    gotestspec.KindBlock,
					Display: "does the thing",
					Status:  gotestspec.StatusFail,
					Output:  []string{"        boot_test.go:9: True failed\n"},
				}},
			}},
		}},
	}}

	stats := gotestspec.CollectStats(packages)
	mustEq(t, "counts (the leaf only)", countsOf(stats), statCounts{Failed: 1, Passed: 0, Total: stats.Behaviors + stats.Tests})

	// Rendering deliberately uses the weaker "has own diagnostic" test, not
	// "failed on its own": it cannot tell this stray t.Log apart from a
	// genuine teardown diagnostic without a machine-readable marker on
	// gotest's own verdicts (follow-up work), and showing a stray log line is
	// preferable to ever hiding a real one. So the summary may still list
	// "Something" beside the leaf; what must not regress is the count above.
	mustEq(t, "leaf failure lines in the summary", strings.Count(summaryOf(packages), "boot_test.go:9: True failed"), 1)
}

// An interior node can carry a genuine diagnostic of its own (AfterAll failed)
// at the same time as a descendant fails independently. The count attributes
// the run's one verdict to the leaf, but the teardown diagnostic must still
// reach the screen: it is the only place that says the database was never
// released.
func (s *InteriorFailureTestSuite) TestCollectStats_CountsLeafButStillRendersInteriorDiagnostic(t *gotest.T, _ *interiorCtx) {
	packages := []*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Boot",
			Status:  gotestspec.StatusFail,
			Output: []string{
				"--- FAIL: TestBootTestSuite (0.02s)\n",
				"    boot_test.go:14: could not release the database\n",
			},
			Children: []*gotestspec.Node{
				{Kind: gotestspec.KindMethod, Display: "Something", Status: gotestspec.StatusPass, Duration: time.Millisecond},
				{
					Kind:    gotestspec.KindMethod,
					Display: "Another",
					Status:  gotestspec.StatusFail,
					Output:  []string{"    --- FAIL: TestBootTestSuite/TestAnother (0.01s)\n"},
					Children: []*gotestspec.Node{{
						Kind:    gotestspec.KindBlock,
						Display: "does the thing",
						Status:  gotestspec.StatusFail,
						Output:  []string{"        boot_test.go:9: True failed\n"},
					}},
				},
			},
		}},
	}}

	mustEq(t, "failed (the leaf only)", gotestspec.CollectStats(packages).Failed, 1)
	mustContain(t, terminalOf(packages), "could not release the database", "the terminal shows the teardown diagnostic")
	mustContain(t, summaryOf(packages), "could not release the database", "the summary shows the teardown diagnostic")
}
