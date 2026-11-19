package gotestrunner_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// tracePkgs is the import path the traced conformance packages live under.
const tracePkgs = "github.com/mvrahden/go-test/tests/conformance/testdata"

// DispatchTraceTestSuite pins the order a run dispatches in: compile intake,
// fixture starts, suite starts and ends, fixture releases.
// Sequential: the dispatch observer is process-wide.
type DispatchTraceTestSuite struct{}

type traceCtx struct {
	mu      sync.Mutex
	events  []gotestrunner.DispatchEvent
	restore func()
	cleanup func()
}

func (s *DispatchTraceTestSuite) BeforeAll(t *gotest.T) { testkit.IsolateTempDir(t) }
func (s *DispatchTraceTestSuite) AfterAll(_ *gotest.T)  {}

func (s *DispatchTraceTestSuite) BeforeEach(t *gotest.T) *traceCtx {
	c := &traceCtx{}
	c.restore = gotestrunner.ExportObserveDispatch(func(e gotestrunner.DispatchEvent) {
		c.mu.Lock()
		c.events = append(c.events, e)
		c.mu.Unlock()
	})
	return c
}

func (s *DispatchTraceTestSuite) AfterEach(_ *gotest.T, c *traceCtx) {
	c.restore()
	if c.cleanup != nil {
		c.cleanup()
	}
}

// run runs pkg in this process and returns the canonical trace and the exit code.
func (c *traceCtx) run(t *gotest.T, pkg string, goTestArgs ...string) (string, int) {
	dir := t.TempDir()
	t.Setenv("GOTEST_CANARY_DIR", dir) // the fixtures stamp markers there, never into the tree
	t.Setenv(protocol.EnvCacheDir, testkit.ChildCacheDir(dir))
	pkgs, err := gotestgen.LoadRunPackages([]string{pkg}, nil)
	gotest.NoError(t, err)
	overlay, cleanup, err := gotestrunner.GenerateOverlay(pkgs.Loaded, pkgs.Broken, false, false, false)
	gotest.NoError(t, err)
	c.cleanup = cleanup
	result, err := gotestrunner.RunPipeline(t.Context(), gotestrunner.PipelineConfig{
		GoTestArgs: goTestArgs, OutputMode: gotestrunner.RunCaptureJSON, Parallel: 8,
		PackageOrder: pkgs.Order,
	}, overlay)
	gotest.NoError(t, err)
	c.mu.Lock()
	events := slices.Clone(c.events)
	c.mu.Unlock()
	return canonicalTrace(events), result.ExitCode
}

func (s *DispatchTraceTestSuite) TestADefaultRun(t *gotest.T, c *traceCtx) {
	trace, exit := c.run(t, "../../tests/conformance/testdata/passing/")
	gotest.Equal(t, 0, exit)
	gotest.NotContains(t, trace, "VIOLATION")
	gotest.Equal(t, "intake "+tracePkgs+"/passing\nup-front []\nscope \"\"\n  bulk "+tracePkgs+"/passing.TestPassingTestSuite [] ran\n", trace)
	gotest.MatchSnapshot(t, trace)
}

func (s *DispatchTraceTestSuite) TestAGroupedRun(t *gotest.T, c *traceCtx) {
	trace, exit := c.run(t, "../../tests/conformance/testdata/groups/")
	gotest.Equal(t, 0, exit)
	gotest.NotContains(t, trace, "VIOLATION")
	p := tracePkgs
	gotest.Equal(t, strings.Join([]string{
		"intake " + p + "/groups",
		"up-front []",
		`scope ""`,
		"  bulk " + p + "/groups.TestADefaultTestSuite [] ran",
		"  bulk " + p + "/groups.TestBDefaultTestSuite [] ran",
		`scope "e2e"`,
		"  bulk " + p + "/groups.TestE2EOneTestSuite [] ran",
		"  bulk " + p + "/groups.TestE2ETwoTestSuite [] ran",
		"  tail " + p + "/groups.TestE2EAloneTestSuite [] ran",
		`scope "load"`,
		"  bulk " + p + "/groups.TestLoadTestSuite [] ran",
		"",
	}, "\n"), trace)
	gotest.MatchSnapshot(t, trace)
}

func (s *DispatchTraceTestSuite) TestAnExclusiveTail(t *gotest.T, c *traceCtx) {
	trace, exit := c.run(t, "../../tests/conformance/testdata/fixaslots/")
	gotest.Equal(t, 0, exit)
	gotest.NotContains(t, trace, "VIOLATION")
	gotest.MatchSnapshot(t, trace)
}

func (s *DispatchTraceTestSuite) TestASharedFixtureReleasedAtItsLastUse(t *gotest.T, c *traceCtx) {
	trace, exit := c.run(t, "../../tests/conformance/testdata/fixalastuse/")
	gotest.Equal(t, 0, exit)
	gotest.NotContains(t, trace, "VIOLATION")
	gotest.MatchSnapshot(t, trace)
}

func (s *DispatchTraceTestSuite) TestGroupsWithFixturesOfTheirOwn(t *gotest.T, c *traceCtx) {
	trace, exit := c.run(t, "../../tests/conformance/testdata/groupfixtures/")
	gotest.Equal(t, 0, exit)
	gotest.NotContains(t, trace, "VIOLATION")
	gotest.MatchSnapshot(t, trace)
}

// TestALateFixtureStartedOnce pins, without a snapshot, that a fixture two
// exclusive suites read is started for the first and kept for the second.
func (s *DispatchTraceTestSuite) TestALateFixtureStartedOnce(t *gotest.T, c *traceCtx) {
	trace, exit := c.run(t, "../../tests/conformance/testdata/tracelatetwice/")
	gotest.Equal(t, 0, exit)
	p, key := tracePkgs+"/tracelatetwice", "["+tracePkgs+"/tracelatetwice.LateSharedFixture]"
	gotest.Equal(t, strings.Join([]string{
		"intake " + p,
		"up-front []",
		`scope ""`,
		"  start " + key + " for exclusive tail",
		"  tail " + p + ".TestAFirstTestSuite " + key + " ran",
		"  tail " + p + ".TestBSecondTestSuite " + key + " ran",
		"free " + key,
		"teardown",
		"",
	}, "\n"), trace)
}

func (s *DispatchTraceTestSuite) TestARunWithNothingToRunEndsClean(t *gotest.T, c *traceCtx) {
	t.Setenv(protocol.EnvCacheDir, t.TempDir())
	overlay, cleanup, err := gotestrunner.GenerateOverlay(nil, nil, false, true, false)
	gotest.NoError(t, err)
	c.cleanup = cleanup
	result, err := gotestrunner.RunPipeline(t.Context(), gotestrunner.PipelineConfig{OutputMode: gotestrunner.RunCaptureJSON}, overlay)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, result.ExitCode)
	gotest.Empty(t, result.CapturedJSON) // a clean empty run captures nothing
	gotest.Empty(t, result.Groups)
}

// traceScope is one scope of a canonical trace.
type traceScope struct {
	bulk map[string]string // suite → its bulk line
	tail []string          // exclusive suites and their starts, in event order
}

// traceSpan is where a suite's events sit in the trace.
type traceSpan struct {
	group     string
	exclusive bool
	keys      []string
	first     int // its first event
	last      int // its last event
	outcome   string
}

func traceKeys(keys []string) string { return "[" + strings.Join(keys, " ") + "]" }

// canonicalTrace renders events as the same text on every run of the same
// code: what interleaves freely is sorted, what runs in a fixed order is not,
// and every broken dispatch rule is named in a VIOLATION line.
func canonicalTrace(events []gotestrunner.DispatchEvent) string {
	var intakes []string
	intakeAt := map[string]int{}
	upFront := "up-front []"
	var order []string
	scopes := map[string]*traceScope{}
	scope := func(g string) *traceScope {
		if scopes[g] == nil {
			scopes[g] = &traceScope{bulk: map[string]string{}}
		}
		return scopes[g]
	}
	seen := map[string]bool{}
	spans := map[string]*traceSpan{}
	var spanOrder []string
	var groupStarts = map[string][]string{}
	var pendingTail []string
	freed := map[string][]int{}
	var freedKeys []string
	startedAt := map[string][]int{} // key → events that start it
	teardownAt := -1
	for i := range events {
		e := events[i]
		id := e.Package + "." + e.Suite
		switch e.Kind {
		case "intake":
			line := "intake " + e.Package
			if e.For == "failed" {
				line += " failed"
			}
			intakes = append(intakes, line)
			intakeAt[e.Package] = i
		case "up-front":
			upFront = "up-front " + traceKeys(e.Keys)
			for _, k := range e.Keys {
				startedAt[k] = append(startedAt[k], i)
			}
		case "start":
			line := "  start " + traceKeys(e.Keys) + " for " + e.For
			for _, k := range e.Keys {
				startedAt[k] = append(startedAt[k], i)
			}
			if g, ok := strings.CutPrefix(e.For, "group "); ok {
				groupStarts[g] = append(groupStarts[g], line)
			} else {
				pendingTail = append(pendingTail, line)
			}
		case "suite-start", "suite-end", "given-up", "stopped":
			if !seen[e.Group] {
				seen[e.Group] = true
				order = append(order, e.Group)
			}
			sp := spans[id]
			if sp == nil {
				sp = &traceSpan{group: e.Group, exclusive: e.Exclusive, keys: e.Keys, first: i}
				spans[id] = sp
				spanOrder = append(spanOrder, id)
				if e.Exclusive {
					sc := scope(e.Group)
					// The suite's own line is filled in once its outcome is known.
					sc.tail = append(append(sc.tail, pendingTail...), "\x00"+id)
					pendingTail = nil
				}
			}
			sp.last = i
			switch e.Kind {
			case "suite-start":
				sp.outcome = "ran"
			case "given-up", "stopped":
				sp.outcome = e.Kind
			}
		case "free":
			for _, k := range e.Keys {
				if len(freed[k]) == 0 {
					freedKeys = append(freedKeys, k)
				}
				freed[k] = append(freed[k], i)
			}
		case "teardown":
			teardownAt = i
		}
	}
	for _, id := range spanOrder {
		sp := spans[id]
		sc := scope(sp.group)
		if sp.exclusive {
			if at := slices.Index(sc.tail, "\x00"+id); at >= 0 {
				sc.tail[at] = "  tail " + id + " " + traceKeys(sp.keys) + " " + sp.outcome
			}
		} else {
			sc.bulk[id] = "  bulk " + id + " " + traceKeys(sp.keys) + " " + sp.outcome
		}
	}

	var b strings.Builder
	slices.Sort(intakes)
	for _, l := range intakes {
		b.WriteString(l + "\n")
	}
	b.WriteString(upFront + "\n")
	for _, g := range order {
		sc := scope(g)
		fmt.Fprintf(&b, "scope %q\n", g)
		for _, l := range groupStarts[g] {
			b.WriteString(l + "\n")
		}
		ids := make([]string, 0, len(sc.bulk))
		for id := range sc.bulk {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			b.WriteString(sc.bulk[id] + "\n")
		}
		for _, l := range sc.tail {
			b.WriteString(l + "\n")
		}
	}
	if len(freedKeys) > 0 {
		slices.Sort(freedKeys)
		b.WriteString("free " + traceKeys(freedKeys) + "\n")
	}
	if teardownAt >= 0 {
		b.WriteString("teardown\n")
	}

	var violations []string
	violate := func(rule, format string, args ...any) {
		violations = append(violations, "VIOLATION "+rule+": "+fmt.Sprintf(format, args...))
	}
	for _, id := range spanOrder {
		sp := spans[id]
		pkg := events[sp.first].Package
		if at, ok := intakeAt[pkg]; sp.outcome == "ran" && (!ok || at > sp.first) {
			violate("R1", "%s starts before its package's intake", id)
		}
	}
	for n := 1; n < len(order); n++ {
		lastEnd, firstStart := -1, len(events)
		for _, sp := range spans {
			if sp.group == order[n-1] {
				lastEnd = max(lastEnd, sp.last)
			}
			if sp.group == order[n] {
				firstStart = min(firstStart, sp.first)
			}
		}
		if lastEnd > firstStart {
			violate("R2", "scope %q starts before scope %q ended", order[n], order[n-1])
		}
	}
	prevExclusive := map[string]*traceSpan{}
	for _, id := range spanOrder {
		sp := spans[id]
		if !sp.exclusive {
			continue
		}
		for otherID, other := range spans {
			if other.group == sp.group && !other.exclusive && other.last > sp.first {
				violate("R3", "%s starts before %s ended", id, otherID)
			}
		}
		if prev := prevExclusive[sp.group]; prev != nil && prev.last > sp.first {
			violate("R3", "%s starts before the exclusive suite before it ended", id)
		}
		prevExclusive[sp.group] = sp
	}
	for _, id := range spanOrder {
		sp := spans[id]
		if sp.outcome != "ran" {
			continue
		}
		for _, k := range sp.keys {
			if !slices.ContainsFunc(startedAt[k], func(at int) bool { return at < sp.first }) {
				violate("R4", "%s starts before %s was started", id, k)
			}
			if slices.ContainsFunc(freed[k], func(at int) bool { return at < sp.first }) {
				violate("R4", "%s starts after %s was freed", id, k)
			}
		}
	}
	for k, ats := range freed {
		if len(ats) > 1 {
			violate("R5", "%s is freed %d times", k, len(ats))
		}
		for id, sp := range spans {
			if slices.Contains(sp.keys, k) && sp.last > ats[0] {
				violate("R5", "%s is freed before %s ended", k, id)
			}
		}
	}
	if teardownAt >= 0 && teardownAt != len(events)-1 {
		violate("R6", "teardown is followed by %s", events[len(events)-1].Kind)
	}
	slices.Sort(violations)
	for _, v := range violations {
		b.WriteString(v + "\n")
	}
	return b.String()
}
