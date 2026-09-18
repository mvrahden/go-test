package gotestrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
)

// CensusCase is one declared unit under the path go test reports it by:
// "Test<Suite>/<Method>", "Fuzz<Suite>_<Method>" or "Benchmark<Suite>/<Method>".
type CensusCase struct {
	Pkg  string
	Path string
}

// DeclaredUnits are the units a run's packages declare after focus and
// exclusion, in source order.
type DeclaredUnits struct {
	Tests      []CensusCase
	Fuzz       []CensusCase
	Benchmarks []CensusCase
}

func appendCases(cases []CensusCase, pkg string, paths []string) []CensusCase {
	for _, p := range paths {
		cases = append(cases, CensusCase{Pkg: pkg, Path: p})
	}
	return cases
}

var (
	testSelectionFlags  = []string{"-run", "-skip", "-list", "-test.run", "-test.skip", "-test.list"}
	benchSelectionFlags = append([]string{"-bench", "-test.bench"}, testSelectionFlags...)
)

// hasSelectionFlag reports whether the go test flags carry one of flags.
func hasSelectionFlag(goTestArgs, flags []string) bool {
	for _, a := range goTestArgs {
		for _, f := range flags {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
		}
	}
	return false
}

// NoteBenchmarksNotRun says how many declared benchmarks a test run leaves
// unexecuted, so silence never implies the packages were fully exercised.
// -bench and -run make no such claim.
func NoteBenchmarksNotRun(w io.Writer, goTestArgs []string, declared int) {
	if declared == 0 || hasSelectionFlag(goTestArgs, benchSelectionFlags) {
		return
	}
	fmt.Fprintf(w, "note: %d benchmark(s) not run — gotest runs tests; use 'gotest bench'\n", declared)
}

// streamEvent is the part of a test2json event the census reads and books.
type streamEvent struct {
	Time    string  `json:"Time,omitempty"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test,omitempty"`
	Output  string  `json:"Output,omitempty"`
	Elapsed float64 `json:"Elapsed,omitempty"`
}

// verdictIndex keeps, from the JSON stream written through it, the last
// verdict of every package, suite, method and fuzz wrapper, the benchmarks
// that reported a result, and the units still running. Deeper subtests are not
// kept.
type verdictIndex struct {
	partial    []byte
	last       map[CensusCase]streamEvent
	order      []CensusCase
	benched    map[CensusCase]bool
	benchOrder []CensusCase
	running    map[CensusCase]bool
	started    []CensusCase
}

func (x *verdictIndex) Write(p []byte) (int, error) {
	x.partial = append(x.partial, p...)
	for {
		i := bytes.IndexByte(x.partial, '\n')
		if i < 0 {
			break
		}
		x.observe(x.partial[:i])
		x.partial = x.partial[i+1:]
	}
	return len(p), nil
}

// verdictMarkers are the substrings a line needs before it is worth decoding.
var verdictMarkers = [][]byte{
	[]byte(`"Action":"pass"`), []byte(`"Action":"fail"`), []byte(`"Action":"skip"`),
	[]byte(`"Action":"bench"`), []byte(" ns/op"), []byte(`"Action":"run"`),
}

var testField = []byte(`"Test":"`)

// rawTestDepth counts the slashes in a line's undecoded Test value. An escaped
// quote ends the count early, so it never exceeds the decoded depth.
func rawTestDepth(line []byte) int {
	_, value, ok := bytes.Cut(line, testField)
	if !ok {
		return 0
	}
	if end := bytes.IndexByte(value, '"'); end >= 0 {
		value = value[:end]
	}
	return bytes.Count(value, []byte("/"))
}

func (x *verdictIndex) observe(line []byte) {
	if !slices.ContainsFunc(verdictMarkers, func(m []byte) bool { return bytes.Contains(line, m) }) || rawTestDepth(line) > 1 {
		return
	}
	var ev streamEvent
	if json.Unmarshal(line, &ev) != nil {
		return
	}
	key := CensusCase{Pkg: ev.Package, Path: ev.Test}
	depth := strings.Count(ev.Test, "/")
	if depth > 1 {
		return
	}
	switch ev.Action {
	case "run":
		if ev.Test != "" {
			if x.running == nil {
				x.running = map[CensusCase]bool{}
			}
			if _, seen := x.running[key]; !seen {
				x.started = append(x.started, key)
			}
			x.running[key] = true
		}
	case "pass", "fail", "skip":
		if x.last == nil {
			x.last = map[CensusCase]streamEvent{}
		}
		if _, seen := x.last[key]; !seen {
			x.order = append(x.order, key)
		}
		ev.Output = ""
		x.last[key] = ev
		x.settle(key)
	}
	if depth == 1 && strings.HasPrefix(ev.Test, protocol.PrefixBenchmark) && benchVerdict(ev) {
		x.settle(key)
		if !x.benched[key] {
			if x.benched == nil {
				x.benched = map[CensusCase]bool{}
			}
			x.benched[key] = true
			x.benchOrder = append(x.benchOrder, key)
		}
	}
}

func (x *verdictIndex) settle(key CensusCase) {
	if x.running[key] {
		x.running[key] = false
	}
}

// runningUnits lists, in start order, the units without a verdict: methods,
// fuzz wrappers and benchmarks, and a suite none of whose units is running (a
// hook hung). A benchmark wrapper never reports a verdict, so it is never
// named itself.
func (x *verdictIndex) runningUnits() []CensusCase {
	busy := map[CensusCase]bool{}
	for _, k := range x.started {
		if suite, _, nested := strings.Cut(k.Path, "/"); nested && x.running[k] && !strings.HasPrefix(suite, protocol.PrefixFuzz) {
			busy[CensusCase{Pkg: k.Pkg, Path: suite}] = true
		}
	}
	var out []CensusCase
	for _, k := range x.started {
		if !x.running[k] || busy[k] {
			continue
		}
		suite, _, nested := strings.Cut(k.Path, "/")
		switch {
		case nested && strings.HasPrefix(suite, protocol.PrefixFuzz):
			continue // a seed; its wrapper is named
		case !nested && strings.HasPrefix(k.Path, protocol.PrefixBenchmark):
			continue
		}
		out = append(out, k)
	}
	return out
}

// benchVerdict reports whether ev settles a benchmark: go test emits no pass
// for one, so its result line counts, as does a fail, skip or bench action.
func benchVerdict(ev streamEvent) bool { //nolint:gocritic // hugeParam: a decoded line
	switch ev.Action {
	case "fail", "skip", "bench":
		return true
	case "output":
		return strings.Contains(ev.Output, " ns/op")
	}
	return false
}

// executedTests lists every Suite/Method with a verdict; a suite skipped as a
// whole appears bare and stands for its methods.
func (x *verdictIndex) executedTests() []CensusCase {
	var out []CensusCase
	for _, k := range x.order {
		if k.Path == "" || strings.HasPrefix(k.Path, protocol.PrefixFuzz) {
			continue
		}
		if strings.Contains(k.Path, "/") || x.last[k].Action == "skip" {
			out = append(out, k)
		}
	}
	return out
}

// executedFuzz lists every fuzz wrapper with a verdict.
func (x *verdictIndex) executedFuzz() []CensusCase {
	var out []CensusCase
	for _, k := range x.order {
		if strings.HasPrefix(k.Path, protocol.PrefixFuzz) && !strings.Contains(k.Path, "/") {
			out = append(out, k)
		}
	}
	return out
}

// missingUnits is declared minus executed, in declaration order; a suite
// skipped as a whole covers its methods.
func missingUnits(declared, executed []CensusCase) []CensusCase {
	ran := make(map[CensusCase]bool, len(executed))
	for _, c := range executed {
		ran[c] = true
	}
	var missing []CensusCase
	for _, d := range declared {
		if ran[d] {
			continue
		}
		if suite, _, ok := strings.Cut(d.Path, "/"); ok && ran[CensusCase{Pkg: d.Pkg, Path: suite}] {
			continue
		}
		missing = append(missing, d)
	}
	return missing
}

// takeCensus holds a green run that ran to completion to the units its
// packages declare. A unit without a verdict makes the run exit 2 and is
// booked into the JSON stream as a failed test, so every renderer names it.
func (c *OutputCollector) takeCensus(cfg PipelineConfig, declared DeclaredUnits, code int, dispatchErr error) int { //nolint:gocritic // hugeParam: stable API
	if c.mode == RunBatchText || code != 0 || dispatchErr != nil {
		return code
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	type group struct {
		kind               string
		declared, executed []CensusCase
	}
	groups := []group{
		{"test", declared.Tests, c.verdicts.executedTests()},
		{"fuzz target", declared.Fuzz, c.verdicts.executedFuzz()},
	}
	flags := testSelectionFlags
	if cfg.Bench {
		groups = []group{{"benchmark", declared.Benchmarks, c.verdicts.benchOrder}}
		flags = benchSelectionFlags
	}
	if hasSelectionFlag(cfg.GoTestArgs, flags) {
		fmt.Fprintln(c.stderr, "note: census skipped under -run/-skip/-list/-bench")
		return code
	}

	var booked []streamEvent
	for _, g := range groups {
		missing := missingUnits(g.declared, g.executed)
		if len(missing) == 0 {
			continue
		}
		fmt.Fprintf(c.stderr, "\nFAIL: census: %d declared %s(s) never ran\n", len(missing), g.kind)
		for _, m := range missing {
			fmt.Fprintf(c.stderr, "  %s %s\n", m.Pkg, m.Path)
		}
		booked = append(booked, bookMissing(g.kind, missing)...)
		booked = append(booked, c.verdicts.suiteFailures(missing)...)
		code = 2
	}
	booked = append(booked, c.verdicts.packageFailures(booked)...)
	w := c.jsonTarget()
	for i := range booked {
		line, _ := json.Marshal(booked[i])
		_, _ = w.Write(append(line, '\n'))
	}
	return code
}

// bookMissing books each missing unit as a test that ran and failed with the
// census as its only output.
func bookMissing(kind string, missing []CensusCase) []streamEvent {
	events := make([]streamEvent, 0, 3*len(missing))
	for _, m := range missing {
		events = append(events,
			streamEvent{Action: "run", Package: m.Pkg, Test: m.Path},
			streamEvent{Action: "output", Package: m.Pkg, Test: m.Path, Output: "    census: declared " + kind + " never ran\n"},
			streamEvent{Action: "fail", Package: m.Pkg, Test: m.Path},
		)
	}
	return events
}

// suiteFailures fails, once each, the suites of the given units that the
// stream started or settled, keeping a verdict's time and duration: the tree
// takes a node's last verdict.
func (x *verdictIndex) suiteFailures(units []CensusCase) []streamEvent {
	seen := map[CensusCase]bool{}
	var fail []streamEvent
	for _, m := range units {
		path, ok := suitePath(m.Path)
		suite := CensusCase{Pkg: m.Pkg, Path: path}
		if !ok || seen[suite] {
			continue
		}
		seen[suite] = true
		v, settled := x.last[suite]
		if _, started := x.running[suite]; settled || started {
			fail = append(fail, streamEvent{Time: v.Time, Action: "fail", Package: suite.Pkg, Test: suite.Path, Elapsed: v.Elapsed})
		}
	}
	return fail
}

// bookDeadline returns the units an expired deadline cut short and books each
// into the JSON stream as failed, with its suite and package. A suite cut short
// is failed whether or not it reported a verdict: it never finished. The text
// run has no stream and names the suites whose process the deadline signalled.
func (c *OutputCollector) bookDeadline(dispatchErr error) []CensusCase {
	if !errors.Is(dispatchErr, context.DeadlineExceeded) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == RunBatchText {
		return slices.Clone(c.cutShort)
	}
	running := c.verdicts.runningUnits()
	booked := make([]streamEvent, 0, 2*len(running))
	for _, r := range running {
		booked = append(booked,
			streamEvent{Action: "output", Package: r.Pkg, Test: r.Path, Output: "    gotest: --timeout expired while this test was running\n"},
			streamEvent{Action: "fail", Package: r.Pkg, Test: r.Path},
		)
	}
	booked = append(booked, c.verdicts.suiteFailures(running)...)
	booked = append(booked, c.verdicts.packageFailures(booked)...)
	w := c.jsonTarget()
	for i := range booked {
		line, _ := json.Marshal(booked[i])
		_, _ = w.Write(append(line, '\n'))
	}
	return running
}

// maxNamedUnits caps the units a one-line message names.
const maxNamedUnits = 5

// unitNames joins units as "pkg path", truncated past maxNamedUnits.
func unitNames(units []CensusCase) string {
	names := make([]string, 0, maxNamedUnits+1)
	for _, u := range units[:min(len(units), maxNamedUnits)] {
		names = append(names, u.Pkg+" "+u.Path)
	}
	if len(units) > maxNamedUnits {
		names = append(names, fmt.Sprintf("… %d more", len(units)-maxNamedUnits))
	}
	return strings.Join(names, ", ")
}

// suitePath is the suite test a unit's verdict folds into; the tree nests a
// fuzz wrapper under its suite.
func suitePath(path string) (string, bool) {
	if suite, _, ok := strings.Cut(path, "/"); ok {
		return suite, true
	}
	if suite, _, ok := protocol.SplitFuzzWrapper(path); ok {
		return "Test" + suite, true
	}
	return "", false
}

// packageFailures fails, once each and after its units, every package the
// booked events name, keeping its time and duration.
func (x *verdictIndex) packageFailures(booked []streamEvent) []streamEvent {
	seen := map[string]bool{}
	var fail []streamEvent
	for i := range booked {
		pkg := booked[i].Package
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		v := x.last[CensusCase{Pkg: pkg}]
		fail = append(fail, streamEvent{Time: v.Time, Action: "fail", Package: pkg, Elapsed: v.Elapsed})
	}
	return fail
}
