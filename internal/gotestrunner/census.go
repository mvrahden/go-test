package gotestrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
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

// mayRunNothing reports whether the user's flags can make a suite's binary
// run no test: a selection flag or -count=0, in gotest's spelling or in the
// binary's own after -args.
func mayRunNothing(goTestArgs, runFlags []string) bool {
	if hasSelectionFlag(goTestArgs, testSelectionFlags) || countsZero(goTestArgs, runFlags) {
		return true
	}
	at := slices.Index(goTestArgs, "-args")
	return at >= 0 && hasSelectionFlag(goTestArgs[at+1:], []string{"-test.bench"})
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
	// Gotest is the event's stamp: read from the stream, and carried by a
	// booked event of a later instance.
	Gotest *protocol.Stamp `json:"Gotest,omitempty"`
}

// verdictIndex keeps, from the JSON stream written through it, the last
// verdict of every package, suite, method and fuzz wrapper, the benchmarks
// that reported a result, and the units still running. Deeper subtests are not
// kept.
type verdictIndex struct {
	// loose reads what no suite process wrote: package verdicts, build
	// output, the runner's own bookings.
	loose      suiteCensus
	last       map[CensusCase]streamEvent
	order      []CensusCase
	benched    map[CensusCase]bool
	benchOrder []CensusCase
	running    map[CensusCase]bool
	started    []CensusCase
	// instance is the run of its top-level name a unit last started in:
	// a suite of both test packages runs twice in one process.
	instance map[CensusCase]int
	// resulted holds the units that printed a result line of their own
	// (--- PASS/FAIL/SKIP); cut holds those the converter failed without one,
	// because the binary ended under them. A cut unit is what a deadline cut
	// short, and is named as such.
	resulted map[CensusCase]bool
	cut      map[CensusCase]bool
	// waiting holds the parallel units paused for a slot and not yet
	// continued: started, but not running.
	waiting map[CensusCase]bool
	// stopped holds, in stream order, the methods of cut-short suites that
	// printed stopEndedLine.
	stopped []CensusCase
}

// suiteCensus is the census's reading of one suite process's output, kept
// apart from every other suite's so their lines may interleave.
type suiteCensus struct {
	x       *verdictIndex
	partial []byte
	// ranNow and cutNow hold the units this output started and cut.
	ranNow, cutNow []CensusCase
	// stoppedNow holds its methods that printed stopEndedLine.
	stoppedNow []CensusCase
}

// suite opens the reading of one suite process's output.
func (x *verdictIndex) suite() *suiteCensus { return &suiteCensus{x: x} }

// stopEndedLine is what the runtime prints on a method's output when the
// method returned after the stop began: it did not finish. No released
// runtime printed an earlier wording, so none other is read.
const stopEndedLine = "run stopping: the stop came while this method ran"

func (x *verdictIndex) Write(p []byte) (int, error) {
	x.loose.x = x
	return x.loose.Write(p)
}

func (s *suiteCensus) Write(p []byte) (int, error) {
	s.partial = append(s.partial, p...)
	for {
		i := bytes.IndexByte(s.partial, '\n')
		if i < 0 {
			break
		}
		s.x.observe(s, s.partial[:i])
		s.partial = s.partial[i+1:]
	}
	return len(p), nil
}

// verdictMarkers are the substrings a line needs before it is worth decoding.
var verdictMarkers = [][]byte{
	[]byte(`"Action":"pass"`), []byte(`"Action":"fail"`), []byte(`"Action":"skip"`),
	[]byte(`"Action":"bench"`), []byte("/op"), []byte(`"Action":"run"`),
	[]byte("--- PASS: "), []byte("--- FAIL: "), []byte("--- SKIP: "),
	[]byte(`"Action":"pause"`), []byte(`"Action":"cont"`),
	[]byte("run stopping: "),
}

var (
	testField      = []byte(`"Test":"`)
	benchTestField = []byte(`"Test":"` + protocol.PrefixBenchmark)
)

// benchResultRe is the "<value> <unit>/op" pair every benchmark result line
// carries, whatever unit go test or ReportMetric printed first.
var benchResultRe = regexp.MustCompile(`\s\S+/op(\s|$)`)

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

func (x *verdictIndex) observe(s *suiteCensus, line []byte) {
	if !slices.ContainsFunc(verdictMarkers, func(m []byte) bool { return bytes.Contains(line, m) }) {
		return
	}
	// Deeper lines are a benchmark's sub-benchmarks, or nothing the census books.
	if rawTestDepth(line) > 1 && !bytes.Contains(line, benchTestField) {
		return
	}
	var ev streamEvent
	if json.Unmarshal(line, &ev) != nil {
		return
	}
	key := CensusCase{Pkg: ev.Package, Path: ev.Test}
	depth := strings.Count(ev.Test, "/")
	if depth > 1 {
		// A method that opens sub-benchmarks prints no result line of its
		// own: the leaves' results are its verdict.
		if strings.HasPrefix(ev.Test, protocol.PrefixBenchmark) && benchVerdict(ev) {
			x.creditBenchmark(CensusCase{Pkg: ev.Package, Path: methodPath(ev.Test)})
		}
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
			if x.instance == nil {
				x.instance = map[CensusCase]int{}
			}
			x.instance[key] = 1
			if ev.Gotest != nil && ev.Gotest.Instance > 1 {
				x.instance[key] = ev.Gotest.Instance
			}
			s.ranNow = append(s.ranNow, key)
			delete(x.resulted, key)
			delete(x.waiting, key)
		}
	case "pause":
		if ev.Test != "" {
			if x.waiting == nil {
				x.waiting = map[CensusCase]bool{}
			}
			x.waiting[key] = true
		}
	case "cont":
		delete(x.waiting, key)
	case "output":
		if ev.Test != "" && isResultLine(ev.Output, ev.Test) {
			if x.resulted == nil {
				x.resulted = map[CensusCase]bool{}
			}
			x.resulted[key] = true
		}
		if depth == 1 && strings.TrimSpace(ev.Output) == stopEndedLine {
			s.stoppedNow = append(s.stoppedNow, key)
		}
	case "pass", "fail", "skip":
		if x.last == nil {
			x.last = map[CensusCase]streamEvent{}
		}
		if _, seen := x.last[key]; !seen {
			x.order = append(x.order, key)
		}
		// A benchmark's fail is booked as its verdict, so it is never counted as cut.
		if ev.Action == "fail" && ev.Test != "" && x.running[key] && !x.waiting[key] && !x.resulted[key] && !strings.HasPrefix(ev.Test, protocol.PrefixBenchmark) {
			if x.cut == nil {
				x.cut = map[CensusCase]bool{}
			}
			x.cut[key] = true
			s.cutNow = append(s.cutNow, key)
		}
		ev.Output = ""
		x.last[key] = ev
		x.settle(key)
	}
	if depth == 1 && strings.HasPrefix(ev.Test, protocol.PrefixBenchmark) && benchVerdict(ev) {
		x.creditBenchmark(key)
	}
}

// isResultLine reports whether output is test's own result line, as go test
// prints it: "--- PASS: <test> (0.00s)", indented by its depth.
func isResultLine(output, test string) bool {
	output = strings.TrimLeft(output, " ")
	for _, verdict := range []string{"PASS", "FAIL", "SKIP"} {
		if strings.HasPrefix(output, "--- "+verdict+": "+test+" (") {
			return true
		}
	}
	return false
}

// end closes the output of one suite process. What its binary ended under
// counts as running or cut only when the run's context stopped it; a binary
// that ended on its own was cut short by no deadline.
func (s *suiteCensus) end(cutShort bool) {
	x := s.x
	if cutShort {
		x.stopped = append(x.stopped, s.stoppedNow...)
	}
	s.stoppedNow = nil
	if !cutShort {
		for _, k := range s.ranNow {
			x.settle(k)
			delete(x.waiting, k)
		}
		for _, k := range s.cutNow {
			delete(x.cut, k)
		}
	}
	// Result lines only tell a cut unit from a finished one while its
	// suite's output is being read.
	for _, k := range s.ranNow {
		delete(x.resulted, k)
	}
	s.ranNow, s.cutNow = nil, nil
}

// creditBenchmark books a benchmark's verdict, once.
func (x *verdictIndex) creditBenchmark(key CensusCase) {
	x.settle(key)
	if !x.benched[key] {
		if x.benched == nil {
			x.benched = map[CensusCase]bool{}
		}
		x.benched[key] = true
		x.benchOrder = append(x.benchOrder, key)
	}
}

// methodPath is the "Benchmark<Suite>/<Method>" prefix of a deeper path.
func methodPath(path string) string {
	parts := strings.SplitN(path, "/", 3)
	return parts[0] + "/" + parts[1]
}

// active reports whether key is still running, or was cut short: failed by
// the converter when the binary ended under it, with no result of its own. A
// unit waiting for a parallel slot is neither.
func (x *verdictIndex) active(key CensusCase) bool {
	return (x.running[key] || x.cut[key]) && !x.waiting[key]
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
		if suite, _, nested := strings.Cut(k.Path, "/"); nested && x.active(k) && !strings.HasPrefix(suite, protocol.PrefixFuzz) {
			busy[CensusCase{Pkg: k.Pkg, Path: suite}] = true
		}
	}
	var out []CensusCase
	for _, k := range x.started {
		if !x.active(k) || busy[k] {
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
// for one, so its result line counts (any "<value> <unit>/op" pair), as does a
// fail, skip or bench action.
func benchVerdict(ev streamEvent) bool { //nolint:gocritic // hugeParam: a decoded line
	switch ev.Action {
	case "fail", "skip", "bench":
		return true
	case "output":
		// The line may arrive in pieces, its name on one chunk and its
		// result on the next: the result is what counts.
		return benchResultRe.MatchString(ev.Output)
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
// packages declare. A unit without a verdict makes the run exit 2. The JSON
// stream books it as a failed test, so every renderer names it; a text run,
// which keeps no stream, judges each suite by its binary's result line.
func (c *OutputCollector) takeCensus(cfg PipelineConfig, declared DeclaredUnits, code int, dispatchErr error) int { //nolint:gocritic // hugeParam: stable API
	text := c.mode == RunBatchText
	if code != 0 || dispatchErr != nil || text && cfg.Bench {
		return code
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	type group struct {
		kind               string
		declared, executed []CensusCase
	}
	executedTests, executedFuzz := c.verdicts.executedTests(), c.verdicts.executedFuzz()
	if text {
		executedTests, executedFuzz = c.ran, c.ran
	}
	if cfg.Only != nil {
		// A rerun is held to what it reran.
		declared = DeclaredUnits{Tests: rerunUnits(declared.Tests, cfg.Only), Fuzz: rerunUnits(declared.Fuzz, cfg.Only)}
	}
	groups := []group{
		{"test", declared.Tests, executedTests},
		{"fuzz target", declared.Fuzz, executedFuzz},
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
	if len(cfg.Groups)+len(cfg.SkipGroups) > 0 {
		fmt.Fprintln(c.stderr, "note: census skipped under --group/--skip-group")
		return code
	}
	if c.censusAside != "" {
		fmt.Fprintf(c.stderr, "note: census skipped under %s\n", c.censusAside)
		return code
	}

	var booked []streamEvent
	notRun := 0
	for _, g := range groups {
		missing := missingUnits(g.declared, g.executed)
		notRun += len(missing)
		if len(missing) == 0 {
			continue
		}
		fmt.Fprintf(c.stderr, "\nFAIL: census: %d declared %s(s) never ran\n", len(missing), g.kind)
		for _, m := range missing {
			fmt.Fprintf(c.stderr, "  %s %s\n", m.Pkg, m.Path)
		}
		c.missing = append(c.missing, missing...)
		booked = append(booked, bookMissing(g.kind, missing)...)
		booked = append(booked, c.verdicts.suiteFailures(missing)...)
		code = 2
	}
	c.notRun = &notRun
	if text {
		return code
	}
	booked = append(booked, c.verdicts.packageFailures(booked)...)
	writeBooked(c.jsonTarget(), booked)
	c.starts.judged(notRun)
	return code
}

// censusAside is why the census does not judge a test run beyond its
// patterns and groups, "" when it does: -count=0, gotest's or the
// binary's, runs nothing, and nor does a binary asked for its usage.
func censusAside(cfg PipelineConfig, runFlags []string) string { //nolint:gocritic // hugeParam: once per run
	switch {
	case countsZero(cfg.GoTestArgs, runFlags):
		return "-count=0"
	case asksUsage(cfg.GoTestArgs):
		return "-args -h"
	}
	return ""
}

// asksUsage reports whether the binary's own words after -args ask it for
// its usage, which it prints instead of running a test.
func asksUsage(goTestArgs []string) bool {
	at := slices.Index(goTestArgs, "-args")
	return at >= 0 && slices.ContainsFunc(goTestArgs[at+1:], func(w string) bool {
		return slices.Contains([]string{"-h", "-help", "--h", "--help", "-test.h", "-test.help"}, w)
	})
}

// rerunUnits are the units of declared that only, as RerunSelection writes
// it, selects: every unit of a package it names whole, else those whose
// levels one of its alternatives matches.
func rerunUnits(declared []CensusCase, only map[string]string) []CensusCase {
	var out []CensusCase
	for _, d := range declared {
		filter, ok := only[d.Pkg]
		if ok && (filter == "" || slices.ContainsFunc(strings.Split(filter, "|"), func(alt string) bool { return matchesLevels(alt, d.Path) })) {
			out = append(out, d)
		}
	}
	return out
}

// matchesLevels reports whether each level of alt matches the same level of
// path; path may go deeper than alt.
func matchesLevels(alt, path string) bool {
	levels, names := strings.Split(alt, "/"), strings.Split(path, "/")
	if len(levels) > len(names) {
		return false
	}
	for i, level := range levels {
		if ok, err := regexp.MatchString(level, names[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

// countsZero reports whether -count=0 is given, in gotest's spelling or in
// the binary's own after -args.
func countsZero(goTestArgs, runFlags []string) bool {
	if extractFlag(runFlags, "-count") == "0" {
		return true
	}
	at := slices.Index(goTestArgs, "-args")
	return at >= 0 && extractFlag(goTestArgs[at+1:], "-test.count") == "0"
}

// reportsResult reports whether a text run's suite binary printed its final
// result line: a binary whose tests never ran, under a TestMain that never
// calls m.Run or an -exec that never starts it, prints none.
func reportsResult(stdout []byte) bool {
	for line := range bytes.SplitSeq(stdout, []byte("\n")) {
		if line := string(bytes.TrimRight(line, "\r")); line == "PASS" || line == "FAIL" {
			return true
		}
	}
	return false
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
			fail = append(fail, streamEvent{Time: v.Time, Action: "fail", Package: suite.Pkg, Test: suite.Path, Elapsed: v.Elapsed, Gotest: x.stamp(suite)})
		}
	}
	return fail
}

// stamp is what a booked event of unit carries: the instance it last
// started in, when that is a later one.
func (x *verdictIndex) stamp(unit CensusCase) *protocol.Stamp {
	if n := x.instance[unit]; n > 1 {
		return &protocol.Stamp{Instance: n}
	}
	return nil
}

// cutShortUnits returns the suites whose process the run's context ended.
func (c *OutputCollector) cutShortUnits() []CensusCase {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.cutShort)
}

// bookDeadline books the units an expired deadline cut short into the JSON
// stream as failed, with its suite and package, and returns those still
// running. A suite cut short is failed whether or not it reported a verdict:
// it never finished. A method the stop ended is booked but not returned: it
// returned, so the run's FAIL line names its suite. The text run has no stream
// and names the suites whose process the deadline signalled.
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
	cut := slices.Clone(running)
	// A method the stop ended returned, maybe passing, but did not finish.
	stopped := map[CensusCase]bool{}
	for _, k := range c.verdicts.stopped {
		if !slices.Contains(cut, k) {
			cut = append(cut, k)
			stopped[k] = true
		}
	}
	booked := make([]streamEvent, 0, 2*len(cut))
	for _, r := range cut {
		booked = append(booked, streamEvent{Action: "output", Package: r.Pkg, Test: r.Path, Output: "    gotest: --timeout expired while this test was running\n", Gotest: c.verdicts.stamp(r)})
		// A unit the converter already failed keeps that verdict.
		if c.verdicts.running[r] || (stopped[r] && c.verdicts.last[r].Action != "fail") {
			booked = append(booked, streamEvent{Action: "fail", Package: r.Pkg, Test: r.Path, Gotest: c.verdicts.stamp(r)})
		}
	}
	booked = append(booked, c.verdicts.suiteFailures(cut)...)
	booked = append(booked, c.verdicts.packageFailures(booked)...)
	writeBooked(c.jsonTarget(), booked)
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

// writeBooked writes the events the run booked itself, stamped like a suite
// process's; an event that carries its stamp keeps it.
func writeBooked(w io.Writer, booked []streamEvent) {
	var block []byte
	for i := range booked {
		line, _ := json.Marshal(booked[i])
		block = append(append(block, line...), '\n')
	}
	_, _ = w.Write(stampBlock(block))
}
